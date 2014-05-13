package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"source.datanerd.us/site-engineering/go_nagios"
)

const (
	API_VERSION = "v1.10"
)

// A struct representing CLI opts that will be passed at runtime
type CliOpts struct {
	BaseUrl string
	CritDataSpace int
	WarnDataSpace int
	CritMetaSpace int
	WarnMetaSpace int
	ImageId string
}

// Information describing the status of a Docker host
type DockerInfo struct {
	Containers     float64
	DriverStatus   [][]string
	DataSpaceUsed  float64
	DataSpaceTotal float64
	MetaSpaceUsed  float64
	MetaSpaceTotal float64
	ImageIsRunning bool
}

type checkArgs struct {
	tag string
	value string
	healthy bool
	appendErrorMessage string
	statusVal nagios.NagiosStatusVal
}


// Describes one container
type Container struct {
	Image		string
	Status		string
}

type HttpResponseFetcher interface {
	Fetch(url string) ([]byte, error)
}

type Fetcher struct{}

// Properly format a Float64 as a string
func float64String(num float64) string {
	return strconv.FormatFloat(num, 'f', 0, 64)
}

// Return a float from a Docker info string for megabytes
func megabytesFloat64(value string) (float64, error) {
	numberStr   := strings.Fields(value)[0]
	number, err := strconv.ParseFloat(numberStr, 64)

	if err != nil {
		return 0.00, err
	}

	return number, nil
}

// Look through a list of driveStatus slices and find the one that matches
func findDriverStatus(key string, driverStatus [][]string) string {
	for _, entry := range(driverStatus) {
		if entry[0] == key {
			return entry[1]
		}
	}

	return ""
}

// Connect to a Docker URL and return the contents as a []byte
func (Fetcher) Fetch(url string) ([]byte, error) {
	response, err := http.Get(url)

	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	return contents, nil
}

// Parses JSON and populates a DockerInfo
func populateInfo(contents []byte, info *DockerInfo) error {
	err := json.Unmarshal(contents, info)
	if err != nil {
		return err
	}

	fields := map[string]*float64{
		"Data Space Used":      &info.DataSpaceUsed,
		"Data Space Total":     &info.DataSpaceTotal,
		"Metadata Space Used":  &info.MetaSpaceUsed,
		"Metadata Space Total": &info.MetaSpaceTotal,
	}

	for key, val := range(fields) {
		entry := findDriverStatus(key, info.DriverStatus)
		if entry == "" {
			return errors.New("Error parsing response from API! Can't find key: " + key)
		}
		*val, err = megabytesFloat64(findDriverStatus(key, info.DriverStatus))
		if err != nil {
			return errors.New("Error parsing response from API! " + err.Error())
		}
	}

	return nil
}

// checkRunningImage looks to see if a container is currently running from a given
// Image Id.
func checkRunningImage(contents []byte, opts *CliOpts) (bool, error) {
	var containers []Container

	err := json.Unmarshal(contents, &containers)
	if err != nil {
		return false, err
	}

	isRunning := false
	for _, container := range containers {
		if strings.HasPrefix(container.Image, opts.ImageId + ":") && strings.HasPrefix(container.Status, "Up") {
			isRunning = true
		}
	}
	return isRunning, nil
}

// fetchInfo retrieves JSON from a Docker host and fills in a DockerInfo
func fetchInfo(fetcher HttpResponseFetcher, opts CliOpts, info *DockerInfo) error {
	contents, err := fetcher.Fetch(opts.BaseUrl + "/" + API_VERSION + "/info")
	if err != nil {
		return err
	}

	err = populateInfo(contents, info)
	if err != nil {
		return err
	}

	if opts.ImageId == "" {
		return nil
	}

	contents, err = fetcher.Fetch(opts.BaseUrl + "/" + API_VERSION + "/containers/json")

	found, err := checkRunningImage(contents, &opts)
	if err != nil {
		return err
	}

	info.ImageIsRunning = found

	return nil
}

// defineChecks returns a list of checks we should run based on CLI flags
func defineChecks(info *DockerInfo, opts *CliOpts) []checkArgs {
	checks := []checkArgs{
		checkArgs{"Meta Space Used",
			float64String(info.MetaSpaceUsed / info.MetaSpaceTotal * 100),
			info.MetaSpaceUsed / info.MetaSpaceTotal * 100 < float64(opts.CritMetaSpace),
			"%",
			nagios.NAGIOS_CRITICAL,
		},
		checkArgs{"Data Space Used",
			float64String(info.DataSpaceUsed / info.DataSpaceTotal * 100),
			info.DataSpaceUsed / info.DataSpaceTotal * 100 < float64(opts.CritDataSpace),
			"%",
			nagios.NAGIOS_CRITICAL,
		},
		checkArgs{"Meta Space Used",
			float64String(info.MetaSpaceUsed / info.MetaSpaceTotal * 100),
			info.MetaSpaceUsed / info.MetaSpaceTotal * 100 < float64(opts.WarnMetaSpace),
			"%",
			nagios.NAGIOS_WARNING,
		},
		checkArgs{"Data Space Used",
			float64String(info.DataSpaceUsed / info.DataSpaceTotal * 100),
			info.DataSpaceUsed / info.DataSpaceTotal * 100 < float64(opts.WarnDataSpace),
			"%",
			nagios.NAGIOS_WARNING,
		},
	}

	if opts.ImageId != "" {
		checks = append(checks,
			checkArgs{"Running Image",
				opts.ImageId,
				info.ImageIsRunning,
				" is not running!",
				nagios.NAGIOS_CRITICAL,
			},
		)
	}

	return checks
}

// Runs a set of checkes and returns an array of statuses
func mapAlertStatuses(info *DockerInfo, opts *CliOpts) []*nagios.NagiosStatus {
	var statuses []*nagios.NagiosStatus

	var check = func(args checkArgs) *nagios.NagiosStatus {
		if !args.healthy {
			return &nagios.NagiosStatus{args.tag + ": " + args.value + args.appendErrorMessage, args.statusVal}
		}
		return nil
	}

	checks := defineChecks(info, opts)

	for _, entry := range(checks) {
		result := check(entry)
		if result != nil {
			statuses = append(statuses, check(entry))
		}
	}

	return statuses
}

func parseCommandLine() *CliOpts {
	var opts CliOpts

	flag.StringVar(&opts.BaseUrl,    "base-url", "http://chi-staging-pool-1:4243/", "The Base URL for the Docker server")
	flag.IntVar(&opts.WarnMetaSpace, "warn-meta-space", 100, "Warning threshold for Metadata Space")
	flag.IntVar(&opts.WarnDataSpace, "warn-data-space", 100, "Warning threshold for Data Space")
	flag.IntVar(&opts.CritMetaSpace, "crit-meta-space", 100, "Critical threshold for Metadata Space")
	flag.IntVar(&opts.CritDataSpace, "crit-data-space", 100, "Critical threshold for Data Space")
	flag.StringVar(&opts.ImageId,    "image-id",         "", "An image ID that must be running on the Docker server")

	flag.Parse()

	return &opts
}

func main() {
	opts := parseCommandLine()

	var fetcher Fetcher
	var info DockerInfo

	err := fetchInfo(fetcher, *opts, &info)
	if err != nil {
		nagios.Critical(err)
	}

	statuses   := mapAlertStatuses(&info, opts)
	baseStatus := nagios.NagiosStatus{float64String(info.Containers) + " containers", 0}

	baseStatus.Aggregate(statuses)
	nagios.ExitWithStatus(&baseStatus)
}
