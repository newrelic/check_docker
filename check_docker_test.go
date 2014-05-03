package main

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

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
		_, err := megabytesFloat64("1024.05 Mb")
		So(err, ShouldBeNil)
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
