package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	dockerlib "github.com/fsouza/go-dockerclient"
	"github.com/newrelic/go_nagios"
	"github.com/shenwei356/util/bytesize"
	"strings"
	"sync"
)

var (
	warnMetaSpace float64
	critMetaSpace float64
	warnDataSpace float64
	critDataSpace float64
	imageId       string
	tlsCertPath   string
	tlsKeyPath    string
	tlsCAPath     string
	endpoint      string
)

func NewCheckDocker(endpoint string) *CheckDocker {
	cd := &CheckDocker{}
	cd.WarnMetaSpace = 100 // defaults
	cd.CritMetaSpace = 100
	cd.WarnDataSpace = 100
	cd.CritDataSpace = 100
	cd.dockerEndpoint = endpoint

	return cd
}

type CheckDocker struct {
	WarnMetaSpace        float64
	CritMetaSpace        float64
	WarnDataSpace        float64
	CritDataSpace        float64
	ImageId              string
	TLSCertPath          string
	TLSKeyPath           string
	TLSCAPath            string
	dockerEndpoint       string
	dockerclient         *dockerlib.Client
	dockerInfoData       *dockerlib.Env
	dockerContainersData []dockerlib.APIContainers
}

func (cd *CheckDocker) setupClient() error {
	var err error

	if cd.TLSCertPath != "" && cd.TLSKeyPath != "" && cd.TLSCAPath != "" {
		cd.dockerclient, err = dockerlib.NewTLSClient(cd.dockerEndpoint, cd.TLSCertPath, cd.TLSKeyPath, cd.TLSCAPath)
	} else {
		cd.dockerclient, err = dockerlib.NewClient(cd.dockerEndpoint)
	}

	return err
}

func (cd *CheckDocker) GetData() error {
	errChan := make(chan error)
	var err error
	var wg sync.WaitGroup

	wg.Add(2)

	go func(cd *CheckDocker, errChan chan error) {
		defer wg.Done()

		cd.dockerInfoData, err = cd.dockerclient.Info()
		if err != nil {
			errChan <- err
		}
	}(cd, errChan)

	go func(cd *CheckDocker, errChan chan error) {
		defer wg.Done()

		cd.dockerContainersData, err = cd.dockerclient.ListContainers(dockerlib.ListContainersOptions{})
		if err != nil {
			errChan <- err
		}
	}(cd, errChan)

	go func() {
		wg.Wait()
		close(errChan)
	}()

	err = <-errChan

	return err
}

func (cd *CheckDocker) getByteSizeDriverStatus(key string) (bytesize.ByteSize, error) {
	var statusInArray [][]string

	err := json.Unmarshal([]byte(cd.dockerInfoData.Get("DriverStatus")), &statusInArray)

	if err != nil {
		return -1, errors.New("Unable to extract DriverStatus info.")
	}

	for _, status := range statusInArray {
		if status[0] == key {
			return bytesize.Parse([]byte(status[1]))
		}
	}

	return -1, errors.New(fmt.Sprintf("DriverStatus does not contain \"%v\"", key))
}

func (cd *CheckDocker) GetDataSpaceUsed() (bytesize.ByteSize, error) {
	return cd.getByteSizeDriverStatus("Data Space Used")
}

func (cd *CheckDocker) GetDataSpaceTotal() (bytesize.ByteSize, error) {
	return cd.getByteSizeDriverStatus("Data Space Total")
}

func (cd *CheckDocker) GetMetaSpaceUsed() (bytesize.ByteSize, error) {
	return cd.getByteSizeDriverStatus("Metadata Space Used")
}

func (cd *CheckDocker) GetMetaSpaceTotal() (bytesize.ByteSize, error) {
	return cd.getByteSizeDriverStatus("Metadata Space Total")
}

func (cd *CheckDocker) IsContainerRunning(imageId string) (dockerlib.APIContainers, bool) {
	for _, container := range cd.dockerContainersData {
		if strings.HasPrefix(container.Image, imageId) && strings.HasPrefix(container.Status, "Up") {
			return container, true
		}
	}
	return dockerlib.APIContainers{}, false
}

func (cd *CheckDocker) IsContainerAGhost(imageId string) (dockerlib.APIContainers, bool) {
	for _, container := range cd.dockerContainersData {
		if strings.HasPrefix(container.Image, imageId) && strings.Contains(container.Status, "Ghost") {
			return container, true
		}
	}
	return dockerlib.APIContainers{}, false
}

func (cd *CheckDocker) CheckMetaSpace(warnThreshold, criticalThreshold float64) *nagios.NagiosStatus {
	usedByteSize, err := cd.GetMetaSpaceUsed()
	if err != nil {
		return &nagios.NagiosStatus{err.Error(), nagios.NAGIOS_CRITICAL}
	}

	totalByteSize, err := cd.GetMetaSpaceTotal()
	if err != nil {
		return &nagios.NagiosStatus{err.Error(), nagios.NAGIOS_CRITICAL}
	}

	percentUsed := float64(usedByteSize/totalByteSize) * 100

	status := &nagios.NagiosStatus{fmt.Sprintf("Metadata Space Usage: %f", percentUsed) + "%", nagios.NAGIOS_OK}

	if percentUsed >= warnThreshold {
		status.Value = nagios.NAGIOS_WARNING
	}
	if percentUsed >= criticalThreshold {
		status.Value = nagios.NAGIOS_CRITICAL
	}

	return status
}

func (cd *CheckDocker) CheckDataSpace(warnThreshold, criticalThreshold float64) *nagios.NagiosStatus {
	usedByteSize, err := cd.GetDataSpaceUsed()
	if err != nil {
		return &nagios.NagiosStatus{err.Error(), nagios.NAGIOS_CRITICAL}
	}

	totalByteSize, err := cd.GetDataSpaceTotal()
	if err != nil {
		return &nagios.NagiosStatus{err.Error(), nagios.NAGIOS_CRITICAL}
	}

	percentUsed := float64(usedByteSize/totalByteSize) * 100

	status := &nagios.NagiosStatus{fmt.Sprintf("Data Space Usage: %f", percentUsed) + "%", nagios.NAGIOS_OK}

	if percentUsed >= warnThreshold {
		status.Value = nagios.NAGIOS_WARNING
	}
	if percentUsed >= criticalThreshold {
		status.Value = nagios.NAGIOS_CRITICAL
	}

	return status
}

func (cd *CheckDocker) CheckImageContainerIsInGoodShape(imageId string) *nagios.NagiosStatus {
	containerRunning, isRunning := cd.IsContainerRunning(imageId)
	containerGhost, isGhost := cd.IsContainerAGhost(imageId)

	if !isRunning {
		return &nagios.NagiosStatus{fmt.Sprintf("Container of image: %v is not running.", imageId), nagios.NAGIOS_CRITICAL}
	}
	if isGhost {
		return &nagios.NagiosStatus{fmt.Sprintf("Container(ID: %v) of image: %v is in ghost state.", containerGhost.ID, imageId), nagios.NAGIOS_CRITICAL}
	}

	return &nagios.NagiosStatus{fmt.Sprintf("Container(ID: %v) of image: %v is in top shape.", containerRunning.ID, imageId), nagios.NAGIOS_OK}
}

func init() {
	flag.StringVar(&endpoint, "base-url", "http://localhost:2375", "The Base URL for the Docker server")
	flag.Float64Var(&warnMetaSpace, "warn-meta-space", 100, "Warning threshold for Metadata Space")
	flag.Float64Var(&critMetaSpace, "crit-meta-space", 100, "Critical threshold for Metadata Space")
	flag.Float64Var(&warnDataSpace, "warn-data-space", 100, "Warning threshold for Data Space")
	flag.Float64Var(&critDataSpace, "crit-data-space", 100, "Critical threshold for Data Space")
	flag.StringVar(&imageId, "image-id", "", "An image ID that must be running on the Docker server")
	flag.StringVar(&tlsCertPath, "tls-cert", "", "Path to TLS cert file.")
	flag.StringVar(&tlsKeyPath, "tls-key", "", "Path to TLS key file.")
	flag.StringVar(&tlsCAPath, "tls-ca", "", "Path to TLS CA file.")
}

func main() {
	flag.Parse()

	cd := NewCheckDocker(endpoint)
	cd.WarnMetaSpace = warnMetaSpace
	cd.CritMetaSpace = critMetaSpace
	cd.WarnDataSpace = warnDataSpace
	cd.CritDataSpace = critDataSpace
	cd.ImageId = imageId
	cd.TLSCertPath = tlsCertPath
	cd.TLSKeyPath = tlsKeyPath
	cd.TLSCAPath = tlsCAPath

	err := cd.setupClient()
	if err != nil {
		nagios.Critical(err)
	}

	err = cd.GetData()
	if err != nil {
		nagios.Critical(err)
	}

	baseStatus := &nagios.NagiosStatus{fmt.Sprintf("Total Containers: %v", len(cd.dockerContainersData)), nagios.NAGIOS_OK}

	statuses := make([]*nagios.NagiosStatus, 0)

	driver := cd.dockerInfoData.Get("Driver")

	// Unfortunately, Metadata Space and Data Space information is only available on devicemapper
	if driver == "devicemapper" {
		statuses = append(statuses, cd.CheckMetaSpace(cd.WarnMetaSpace, cd.CritMetaSpace))
		statuses = append(statuses, cd.CheckDataSpace(cd.WarnDataSpace, cd.CritDataSpace))
	}

	if cd.ImageId != "" {
		statuses = append(statuses, cd.CheckImageContainerIsInGoodShape(cd.ImageId))
	}

	baseStatus.Aggregate(statuses)
	nagios.ExitWithStatus(baseStatus)
}
