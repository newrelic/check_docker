package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/newrelic/go_nagios"
)

const (
	API_VERSION = "v1.10"
)

// A struct representing CLI opts that will be passed at runtime
type CliOpts struct {
	BaseUrl       string
	CritDataSpace int
	WarnDataSpace int
	CritMetaSpace int
	WarnMetaSpace int
	ImageId       string
	GhostsStatus  int
}

// Information describing the status of a Docker host
type DockerInfo struct {
	Containers     float64
	Driver         string
	DriverStatus   [][]string
	DataSpaceUsed  float64
	DataSpaceTotal float64
	MetaSpaceUsed  float64
	MetaSpaceTotal float64
	ImageIsRunning bool
	GhostCount     int
}

// Used internally to build lists of checks to run
type checkArgs struct {
	tag                string
	value              string
	healthy            bool
	appendErrorMessage string
	statusVal          nagios.NagiosStatusVal
}

// Describes one container
type Container struct {
	Image  string
	Status string
}

// An interface to request things from the Web
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
	numberStr := strings.Fields(value)[0]
	number, err := strconv.ParseFloat(numberStr, 64)

	if err != nil {
		return 0.00, err
	}

	return number, nil
}

// Look through a list of driveStatus slices and find the one that matches
func findDriverStatus(key string, driverStatus [][]string) string {
	for _, entry := range driverStatus {
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

	for key, val := range fields {
		entry := findDriverStatus(key, info.DriverStatus)
		if entry != "" {
			*val, err = megabytesFloat64(findDriverStatus(key, info.DriverStatus))
			if err != nil {
				return errors.New("Error parsing response from API! " + err.Error())
			}
		}
	}

	return nil
}

// checkRunningContainers looks to see if a container is currently running from a given
// Image Id.
func checkRunningContainers(contents []byte, opts *CliOpts) (bool, int, error) {
	var containers []Container

	err := json.Unmarshal(contents, &containers)
	if err != nil {
		return false, 0, err
	}

	isRunning := false
	ghostCount := 0
	for _, container := range containers {
		if strings.HasPrefix(container.Image, opts.ImageId+":") && strings.HasPrefix(container.Status, "Up") {
			isRunning = true
		} else if strings.Contains(container.Status, "Ghost") {
			ghostCount += 1
		}
	}
	return isRunning, ghostCount, nil
}

// fetchInfo retrieves JSON from a Docker host and fills in a DockerInfo
func fetchInfo(fetcher HttpResponseFetcher, opts CliOpts, info *DockerInfo) error {
	var wg sync.WaitGroup
	wg.Add(2)

	var err, err2 error
	var imageFound bool
	var ghostCount int

	// `info` is handled by this goroutine but not the other
	go func() {
		var infoResult []byte
		infoResult, err = fetcher.Fetch(opts.BaseUrl + "/" + API_VERSION + "/info")
		if err == nil {
			err = populateInfo(infoResult, info)
		}
		wg.Done()
	}()

	go func() {
		var containersResult []byte
		containersResult, err2 = fetcher.Fetch(opts.BaseUrl + "/" + API_VERSION + "/containers/json")
		if err2 == nil {
			imageFound, ghostCount, err2 = checkRunningContainers(containersResult, &opts)
		}
		wg.Done()
	}()

	wg.Wait()

	if err != nil {
		return err
	}

	if err2 != nil {
		return err2
	}

	info.ImageIsRunning = imageFound
	info.GhostCount = ghostCount

	return nil
}

// defineChecks returns a list of checks we should run based on CLI flags
func defineChecks(info *DockerInfo, opts *CliOpts) []checkArgs {
	checks := make([]checkArgs, 0)

	if info.Driver == "devicemapper" {
		checks = append(checks,
			checkArgs{"Meta Space Used",
				float64String(info.MetaSpaceUsed / info.MetaSpaceTotal * 100),
				info.MetaSpaceUsed/info.MetaSpaceTotal*100 < float64(opts.CritMetaSpace),
				"%",
				nagios.NAGIOS_CRITICAL,
			},
			checkArgs{"Data Space Used",
				float64String(info.DataSpaceUsed / info.DataSpaceTotal * 100),
				info.DataSpaceUsed/info.DataSpaceTotal*100 < float64(opts.CritDataSpace),
				"%",
				nagios.NAGIOS_CRITICAL,
			},
			checkArgs{"Meta Space Used",
				float64String(info.MetaSpaceUsed / info.MetaSpaceTotal * 100),
				info.MetaSpaceUsed/info.MetaSpaceTotal*100 < float64(opts.WarnMetaSpace),
				"%",
				nagios.NAGIOS_WARNING,
			},
			checkArgs{"Data Space Used",
				float64String(info.DataSpaceUsed / info.DataSpaceTotal * 100),
				info.DataSpaceUsed/info.DataSpaceTotal*100 < float64(opts.WarnDataSpace),
				"%",
				nagios.NAGIOS_WARNING,
			},
			checkArgs{"Ghost Containers",
				strconv.Itoa(info.GhostCount),
				info.GhostCount == 0,
				"",
				nagios.NagiosStatusVal(opts.GhostsStatus),
			},
		)
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

	for _, entry := range checks {
		result := check(entry)
		if result != nil {
			statuses = append(statuses, check(entry))
		}
	}

	return statuses
}

// parseCommandLine parses the flags passed on the CLI
func parseCommandLine() *CliOpts {
	var opts CliOpts

	flag.StringVar(&opts.BaseUrl, "base-url", "", "The Base URL for the Docker server")
	flag.IntVar(&opts.WarnMetaSpace, "warn-meta-space", 100, "Warning threshold for Metadata Space")
	flag.IntVar(&opts.WarnDataSpace, "warn-data-space", 100, "Warning threshold for Data Space")
	flag.IntVar(&opts.CritMetaSpace, "crit-meta-space", 100, "Critical threshold for Metadata Space")
	flag.IntVar(&opts.CritDataSpace, "crit-data-space", 100, "Critical threshold for Data Space")
	flag.StringVar(&opts.ImageId, "image-id", "", "An image ID that must be running on the Docker server")
	flag.IntVar(&opts.GhostsStatus, "ghosts-status", 1, "If ghosts are present, treat as this status")

	flag.Parse()

	return &opts
}

func main() {
	opts := parseCommandLine()

	if opts.BaseUrl == "" {
		nagios.Critical(errors.New("-base-url must be supplied"))
	}

	var fetcher Fetcher
	var info DockerInfo

	err := fetchInfo(fetcher, *opts, &info)
	if err != nil {
		nagios.Critical(err)
	}

	statuses := mapAlertStatuses(&info, opts)

	baseStatus := nagios.NagiosStatus{float64String(info.Containers) + " containers", 0}
	perfdata := nagios.NagiosPerformanceVal{"containers", float64String(info.Containers), "", "", "", "", ""}
	perfStatus := nagios.NagiosStatusWithPerformanceData{&baseStatus, perfdata}

	perfStatus.Aggregate(statuses)
	perfStatus.NagiosExit()
}
