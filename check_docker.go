package main

import (
	"errors"
	"flag"
	"fmt"
	dockerlib "github.com/fsouza/go-dockerclient"
	"github.com/newrelic/go_nagios"
	"github.com/shenwei356/util/bytesize"
	"strings"
	"sync"
)

func NewCheckDocker(endpoint string) (*CheckDocker, error) {
	var err error

	cd := &CheckDocker{}
	cd.ImageIds = make([]string, 0)
	cd.ContainerNames = make([]string, 0)
	cd.WarnMetaSpace = 100 // defaults
	cd.CritMetaSpace = 100
	cd.WarnDataSpace = 100
	cd.CritDataSpace = 100

	if endpoint != "" {
		err = cd.setupClient(endpoint)
	}

	return cd, err
}

type CheckDocker struct {
	WarnMetaSpace        float64
	CritMetaSpace        float64
	WarnDataSpace        float64
	CritDataSpace        float64
	ImageIds             []string
	ContainerNames       []string
	TLSCertPath          string
	TLSKeyPath           string
	TLSCAPath            string
	dockerclient         *dockerlib.Client
	dockerInfoData       *dockerlib.DockerInfo
	dockerContainersData []dockerlib.APIContainers
}

func (cd *CheckDocker) setupClient(endpoint string) error {
	var err error

	if cd.TLSCertPath != "" && cd.TLSKeyPath != "" && cd.TLSCAPath != "" {
		cd.dockerclient, err = dockerlib.NewTLSClient(endpoint, cd.TLSCertPath, cd.TLSKeyPath, cd.TLSCAPath)
	} else {
		cd.dockerclient, err = dockerlib.NewClient(endpoint)
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
	statusInArray := cd.dockerInfoData.DriverStatus
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

func (cd *CheckDocker) IsNamedContainerRunning(containerName string) (dockerlib.APIContainers, bool) {
	for _, container := range cd.dockerContainersData {
		for _, name := range container.Names {
			// Container names start with a slash for some reason, maybe a bug?
			// Remove the leading / only if it's found so this doesn't break in
			// the future.
			if strings.HasPrefix(name, "/") {
				name = name[1:]
			}
			if name == containerName {
				if strings.HasPrefix(container.Status, "Up") {
					return container, true
				} else {
					return dockerlib.APIContainers{}, false
				}
			}
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
		return &nagios.NagiosStatus{fmt.Sprintf("Container(ID: %v) of image: %v is in ghost state.", containerGhost.ID[:12], imageId), nagios.NAGIOS_CRITICAL}
	}

	return &nagios.NagiosStatus{fmt.Sprintf("Container(ID: %v) of image: %v is in top shape.", containerRunning.ID[:12], imageId), nagios.NAGIOS_OK}
}

func (cd *CheckDocker) CheckNamedContainerIsInGoodShape(containerName string) *nagios.NagiosStatus {
	container, isRunning := cd.IsNamedContainerRunning(containerName)
	_, isGhost := cd.IsContainerAGhost(container.ID)

	if !isRunning {
		return &nagios.NagiosStatus{fmt.Sprintf("Container named: %v is not running.", containerName), nagios.NAGIOS_CRITICAL}
	}
	if isGhost {
		return &nagios.NagiosStatus{fmt.Sprintf("Container(ID: %v) named: %v is in ghost state.", container.ID[:12], containerName), nagios.NAGIOS_CRITICAL}
	}

	return &nagios.NagiosStatus{fmt.Sprintf("Container(ID: %v) named: %v is in top shape.", container.ID[:12], containerName), nagios.NAGIOS_OK}
}

func main() {
	cd, err := NewCheckDocker("")
	if err != nil {
		nagios.Critical(err)
	}

	var dockerEndpoint string
	var imageIds multiStringArg
	var contNames multiStringArg

	flag.StringVar(&dockerEndpoint, "base-url", "http://localhost:2375", "The Base URL for the Docker server")
	flag.Float64Var(&cd.WarnMetaSpace, "warn-meta-space", 100, "Warning threshold for Metadata Space")
	flag.Float64Var(&cd.CritMetaSpace, "crit-meta-space", 100, "Critical threshold for Metadata Space")
	flag.Float64Var(&cd.WarnDataSpace, "warn-data-space", 100, "Warning threshold for Data Space")
	flag.Float64Var(&cd.CritDataSpace, "crit-data-space", 100, "Critical threshold for Data Space")
	flag.Var(&imageIds, "image-id", "An image ID that must be running on the Docker server")
	flag.Var(&contNames, "container-name", "The name of a container that must be running on the Docker server")
	flag.StringVar(&cd.TLSCertPath, "tls-cert", "", "Path to TLS cert file.")
	flag.StringVar(&cd.TLSKeyPath, "tls-key", "", "Path to TLS key file.")
	flag.StringVar(&cd.TLSCAPath, "tls-ca", "", "Path to TLS CA file.")

	flag.Parse()

	err = cd.setupClient(dockerEndpoint)
	if err != nil {
		nagios.Critical(err)
	}

	if imageIds != nil && len(imageIds) > 0 {
		cd.ImageIds = imageIds
	}
	if contNames != nil && len(contNames) > 0 {
		cd.ContainerNames = contNames
	}

	err = cd.GetData()
	if err != nil {
		nagios.Critical(err)
	}

	baseStatus := &nagios.NagiosStatus{fmt.Sprintf("Total Containers: %v", len(cd.dockerContainersData)), nagios.NAGIOS_OK}

	statuses := make([]*nagios.NagiosStatus, 0)

	driver := cd.dockerInfoData.Driver

	// Unfortunately, Metadata Space and Data Space information is only available on devicemapper
	if driver == "devicemapper" {
		statuses = append(statuses, cd.CheckMetaSpace(cd.WarnMetaSpace, cd.CritMetaSpace))
		statuses = append(statuses, cd.CheckDataSpace(cd.WarnDataSpace, cd.CritDataSpace))
	}

	if len(cd.ImageIds) > 0 {
		for _, imgId := range(cd.ImageIds) {
			statuses = append(statuses, cd.CheckImageContainerIsInGoodShape(imgId))
		}
	}

	if len(cd.ContainerNames) > 0 {
		for _, cName := range(cd.ContainerNames) {
			statuses = append(statuses, cd.CheckNamedContainerIsInGoodShape(cName))
		}
	}

	baseStatus.Aggregate(statuses)
	nagios.ExitWithStatus(baseStatus)
}
