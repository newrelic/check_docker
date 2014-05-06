package main

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"source.datanerd.us/site-engineering/go_nagios"
)

var jsonFromApi []byte = []byte(
	`{
		"DriverStatus": [
			["Data Space Used", "20.0 mb"],
			["Data Space Total", "1000.0 mb"],
			["Metadata Space Used", "15.0 mb"],
			["Metadata Space Total", "200.0 mb"]
		]
	}`,
)


type stubFetcher struct{}

func (fetcher stubFetcher) Fetch(url string) ([]byte, error) {
	return jsonFromApi, nil
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
	Convey("Correctly parses the JSON and populates the DockerInfo", t, func() {
		var info DockerInfo
		err := populateInfo(jsonFromApi, &info)

		So(err, ShouldBeNil)
		So(info.DataSpaceUsed, ShouldEqual, 20.0)
	})

	Convey("Returns an intelligent error when the key is not found", t, func() {
		var info DockerInfo
		err := populateInfo([]byte(`{}`), &info)

		So(err.Error(), ShouldContainSubstring, "Can't find key: Data Space Used")
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
}

func TestMapAlertStatuses(t *testing.T) {
	opts := CliOpts{
		CritMetaSpace: 6.0,
		WarnDataSpace: 5.0,
	}

	Convey("When handed a DockerInfo, returns a list of check results", t, func() {
		var info DockerInfo
		populateInfo(jsonFromApi, &info)
		results := mapAlertStatuses(&info, &opts)

		So(results[0], ShouldHaveTheSameNagiosStatusAs, &nagios.NagiosStatus{"Meta Space Used: 8%", nagios.NAGIOS_CRITICAL})
		So(results[2], ShouldHaveTheSameNagiosStatusAs, &nagios.NagiosStatus{"Meta Space Used: 8%", nagios.NAGIOS_WARNING})
	})

	Convey("Produces output that can properly be aggregated by Nagios", t, func() {
		var info DockerInfo
		populateInfo(jsonFromApi, &info)
		results := mapAlertStatuses(&info, &opts)

		status := &nagios.NagiosStatus{"Chaucer", nagios.NAGIOS_UNKNOWN}
		status.Aggregate(results)
		expected := &nagios.NagiosStatus{"Chaucer - Meta Space Used: 8% - Data Space Used: 2% - Meta Space Used: 8%", nagios.NAGIOS_UNKNOWN}

		So(status, ShouldHaveTheSameNagiosStatusAs, expected)
	})
}

func ShouldHaveTheSameNagiosStatusAs(actual interface{}, expected ...interface{}) string {
	wanted := expected[0].(*nagios.NagiosStatus)
	got    := actual.(*nagios.NagiosStatus)

	if got.Value != wanted.Value || got.Message != wanted.Message {
		return "expected:\n" + fmt.Sprintf("%#v", wanted) + "\n\ngot:\n" + fmt.Sprintf("%#v", got)
	}

	return ""
}
