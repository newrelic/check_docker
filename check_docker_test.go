package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/newrelic/go_nagios"
	. "github.com/smartystreets/goconvey/convey"
)

var devicemapperInfoJsonFromApi []byte = []byte(
	`{
		"Driver": "devicemapper",
		"DriverStatus": [
			["Data Space Used", "20.0 mb"],
			["Data Space Total", "1000.0 mb"],
			["Metadata Space Used", "15.0 mb"],
			["Metadata Space Total", "200.0 mb"]
		]
	}`,
)

var aufsInfoJsonFromApi []byte = []byte(
	`
	{
		"Containers": 0,
		"Debug": 0,
		"Driver": "aufs",
		"DriverStatus": [
			["Root Dir","/usr/local/lib/docker/aufs"],
			["Dirs","0"]
		],
		"ExecutionDriver": "native-0.2",
		"IPv4Forwarding": 1,
		"Images": 0,
		"IndexServerAddress": "https://index.docker.io/v1/",
		"InitPath": "/usr/bin/docker",
		"InitSha1": "",
		"KernelVersion": "3.8.0-35-generic",
		"MemoryLimit": 1,
		"NEventsListener": 0,
		"NFd": 11,
		"NGoroutines": 11,
		"Sockets": [
			"tcp://0.0.0.0:4243",
			"tcp://0.0.0.0:2375",
			"unix:///var/run/docker.sock"
		],
		"SwapLimit":1
	}`,
)

var containersJsonFromApi []byte = []byte(
	`[
	  {
	    "Command": "script/run ",
	    "Created": 1399681210,
	    "Id": "ded464bf7dfb978b6b101c289a06b59a1c64435b3b7e70c97e6876ceb2a9a159",
	    "Image": "testing:b969c9317cc60c389162cbdb2999806ef9b9666b",
	    "Names": [
	      "/insane_franklin"
	    ],
	    "Ports": [
	      {
	        "IP": "0.0.0.0",
	        "PrivatePort": 80,
	        "PublicPort": 8485,
	        "Type": "tcp"
	      }
	    ],
	    "Status": "Up 3 days"
	  },
	  {
	    "Command": "script/run ",
	    "Created": 1399681124,
	    "Id": "a64bba6cd0dbfb9b1bc1880f38d138a1c69a929853dcfca72314d1242e00017c",
	    "Image": "real:b969c9317cc60c389162cbdb2999806ef9b9666b",
	    "Names": [
	      "/sad_ptolemy"
	    ],
	    "Ports": [
	      {
	        "IP": "0.0.0.0",
	        "PrivatePort": 80,
	        "PublicPort": 80,
	        "Type": "tcp"
	      }
	    ],
	    "Status": "Exit 0"
	  },
	  {
	    "Command": "script/run ",
	    "Created": 1399681124,
	    "Id": "2938378cd0dbfb9b1bc1880f38d138a1c69a929853dcfca72314d1242e00017c",
	    "Image": "busted:b969c9317cc60c389162cbdb2999806ef9b9666b",
	    "Names": [
	      "/happy_galileo"
	    ],
	    "Ports": [
	      {
	        "IP": "0.0.0.0",
	        "PrivatePort": 80,
	        "PublicPort": 8999,
	        "Type": "tcp"
	      }
	    ],
	    "Status": "Ghost"
	  }
	]`,
)

type stubFetcher struct{}

func (fetcher stubFetcher) Fetch(url string) ([]byte, error) {
	if strings.Contains(url, "/info") {
		return devicemapperInfoJsonFromApi, nil
	}

	if strings.Contains(url, "/containers") {
		return containersJsonFromApi, nil
	}

	return nil, errors.New("Don't recognize URL: " + url)
}

func TestFloat64String(t *testing.T) {
	Convey("Converts a float to a formatted string with no decimals", t, func() {
		So(float64String(1.2), ShouldEqual, "1")
	})
}

func TestMegabytesFloat64(t *testing.T) {
	Convey("Extracts the float from a Docker megabytes measurement string", t, func() {
		result, _ := megabytesFloat64("1024.05 Mb")
		So(result, ShouldEqual, 1024.05)
	})

	Convey("Returns an error when not parseable", t, func() {
		_, err := megabytesFloat64("1024.05mb")
		So(err.Error(), ShouldContainSubstring, "invalid syntax")
	})
}

func TestFindDriverStatus(t *testing.T) {
	driverStatuses := [][]string{
		[]string{"Key", "Value"},
		[]string{"Key2", "Value2"},
	}

	Convey("Looks up values from a slice by the first element", t, func() {
		So(findDriverStatus("Key", driverStatuses), ShouldEqual, "Value")
		So(findDriverStatus("Key2", driverStatuses), ShouldEqual, "Value2")
	})

	Convey("Returns empty on failure", t, func() {
		So(findDriverStatus("KeyFoo", driverStatuses), ShouldEqual, "")
	})
}

func TestPopulateDriverInfo(t *testing.T) {
	Convey("Correctly parses devicemapper /info JSON and populates the DockerInfo", t, func() {
		var info DockerInfo
		err := populateInfo(devicemapperInfoJsonFromApi, &info)

		So(err, ShouldBeNil)
		So(info.Driver, ShouldEqual, "devicemapper")
		So(info.DataSpaceUsed, ShouldEqual, 20.0)
	})

	Convey("Correctly parses AUFS /info JSON and populates the DockerInfo", t, func() {
		var info DockerInfo
		err := populateInfo(aufsInfoJsonFromApi, &info)

		So(err, ShouldBeNil)
		So(info.Driver, ShouldEqual, "aufs")
	})
}

func TestCheckRunningContainers(t *testing.T) {
	Convey("Searches a JSON blob to find an image with a specified tag", t, func() {
		running, _, err := checkRunningContainers(containersJsonFromApi, &CliOpts{ImageId: "testing"})

		So(err, ShouldBeNil)
		So(running, ShouldBeTrue)
	})

	Convey("Correctly identifies when the tag is missing", t, func() {
		running, _, err := checkRunningContainers(containersJsonFromApi, &CliOpts{ImageId: "Shakespeare"})

		So(err, ShouldBeNil)
		So(running, ShouldBeFalse)
	})

	Convey("Bubbles up errors from the Json library", t, func() {
		running, _, err := checkRunningContainers([]byte("-"), &CliOpts{ImageId: "Shakespeare"})

		So(err, ShouldNotBeNil)
		So(running, ShouldBeFalse)
	})

	Convey("Identifies ghost containers", t, func() {
		_, ghosts, err := checkRunningContainers(containersJsonFromApi, &CliOpts{ImageId: "Shakespeare"})

		So(err, ShouldBeNil)
		So(ghosts, ShouldEqual, 1)
	})
}

func TestFetchInfo(t *testing.T) {
	Convey("Can fetch info using a Fetcher and populate a DockerInfo", t, func() {
		var info DockerInfo
		var stub stubFetcher
		err := fetchInfo(stub, CliOpts{}, &info)

		So(err, ShouldBeNil)
		So(info.DataSpaceUsed, ShouldEqual, 20.0)
	})

	Convey("Populates the ImageIsRunning field when told to by CLI flags", t, func() {
		var info DockerInfo
		var stub stubFetcher
		err := fetchInfo(stub, CliOpts{ImageId: "testing"}, &info)

		So(err, ShouldBeNil)
		So(info.ImageIsRunning, ShouldBeTrue)
	})
}

func TestMapAlertStatuses(t *testing.T) {
	opts := CliOpts{
		CritMetaSpace: 6.0,
		WarnDataSpace: 5.0,
	}

	Convey("Given AUFS /info JSON, when handed a DockerInfo, don't break", t, func() {
		var info DockerInfo
		err := populateInfo(aufsInfoJsonFromApi, &info)

		So(err, ShouldBeNil)

		results := mapAlertStatuses(&info, &opts)
		So(len(results), ShouldEqual, 0)
	})

	Convey("Given devicemapper /info JSON, when handed a DockerInfo, returns a list of check results", t, func() {
		var info DockerInfo
		populateInfo(devicemapperInfoJsonFromApi, &info)
		results := mapAlertStatuses(&info, &opts)

		So(results[0], ShouldHaveTheSameNagiosStatusAs, &nagios.NagiosStatus{"Meta Space Used: 8%", nagios.NAGIOS_CRITICAL})
		So(results[2], ShouldHaveTheSameNagiosStatusAs, &nagios.NagiosStatus{"Meta Space Used: 8%", nagios.NAGIOS_WARNING})
	})

	Convey("Produces output that can properly be aggregated by Nagios", t, func() {
		var info DockerInfo
		populateInfo(devicemapperInfoJsonFromApi, &info)
		results := mapAlertStatuses(&info, &opts)

		status := &nagios.NagiosStatus{"Chaucer", nagios.NAGIOS_UNKNOWN}
		status.Aggregate(results)
		expected := &nagios.NagiosStatus{"Chaucer - Meta Space Used: 8% - Data Space Used: 2% - Meta Space Used: 8%", nagios.NAGIOS_UNKNOWN}

		So(status, ShouldHaveTheSameNagiosStatusAs, expected)
	})

	Convey("Correctly handles the exit status when ghosts are present", t, func() {
		var info DockerInfo
		var stub stubFetcher
		opts := CliOpts{
			CritMetaSpace: 100,
			CritDataSpace: 100,
			GhostsStatus:  2,
		}
		fetchInfo(stub, opts, &info)
		results := mapAlertStatuses(&info, &opts)

		expected := &nagios.NagiosStatus{"Ghost Containers: 1", nagios.NAGIOS_CRITICAL}
		So(results[2], ShouldHaveTheSameNagiosStatusAs, expected)
	})
}

func ShouldHaveTheSameNagiosStatusAs(actual interface{}, expected ...interface{}) string {
	wanted := expected[0].(*nagios.NagiosStatus)
	got := actual.(*nagios.NagiosStatus)

	if got.Value != wanted.Value || got.Message != wanted.Message {
		return "expected:\n" + fmt.Sprintf("%#v", wanted) + "\n\ngot:\n" + fmt.Sprintf("%#v", got)
	}

	return ""
}
