package main

import (
	"github.com/shenwei356/util/bytesize"
	"os"
	"testing"
)

func init() {
	if os.Getenv("DOCKER_IMAGE") == "" {
		println("You must set DOCKER_IMAGE to test image related things.")
	}
}

func TestByteSizeDoTheRightThing(t *testing.T) {
	value, err := bytesize.Parse([]byte("1 kb"))
	if err != nil {
		t.Errorf("Failed to parse byte size. Error: %v", err)
	}
	if float64(value) != 1024 {
		t.Errorf("Failed to parse byte size correctly. Value: %v", float64(value))
	}

	value, err = bytesize.Parse([]byte("1 mB"))
	if err != nil {
		t.Errorf("Failed to parse byte size. Error: %v", err)
	}
	if float64(value) != 1024*1024 {
		t.Errorf("Failed to parse byte size correctly. Value: %v", float64(value))
	}

	value, err = bytesize.Parse([]byte("1 GB"))
	if err != nil {
		t.Errorf("Failed to parse byte size. Error: %v", err)
	}
	if float64(value) != 1024*1024*1024 {
		t.Errorf("Failed to parse byte size correctly. Value: %v", float64(value))
	}
}

func NewCheckDockerForTest(t *testing.T) *CheckDocker {
	endpoint := os.Getenv("DOCKER_HOST")
	if endpoint == "" {
		endpoint = "http://localhost:2375"
	}

	cd, err := NewCheckDocker(endpoint)
	if err != nil {
		t.Fatalf("Failed to initialize CheckDocker. Error: %v", err)
	}

	tlsVerify := os.Getenv("DOCKER_TLS_VERIFY")
	if tlsVerify == "1" {
		certPath := os.Getenv("DOCKER_CERT_PATH")
		cd.TLSCertPath = certPath + "/cert.pem"
		cd.TLSKeyPath = certPath + "/key.pem"
		cd.TLSCAPath = certPath + "/ca.pem"
	}

	err = cd.setupClient()
	if err != nil {
		t.Fatalf("Failed to initialize docker client. Error: %v", err)
	}

	if cd.dockerclient == nil {
		t.Fatalf("Failed to initialize docker client. You must have a docker server (defined in DOCKER_HOST) to run test.")
	}

	err = cd.GetData()
	if err != nil {
		t.Fatalf("Unable to get data from docker server. Error: %v", err)
	}

	return cd
}

func TestNewCheckDocker(t *testing.T) {
	NewCheckDockerForTest(t)
}

func TestGetByteSizeDriverStatus(t *testing.T) {
	cd := NewCheckDockerForTest(t)

	driver := cd.dockerInfoData.Get("Driver")

	for _, key := range []string{"Data Space Used", "Data Space Total", "Metadata Space Used", "Metadata Space Total"} {
		if driver == "aufs" {
			_, err := cd.getByteSizeDriverStatus(key)
			if err == nil {
				t.Errorf("%v does not provide this information: %v.", driver, key)
			}
		} else if driver == "devicemapper" {
			byteSizeValue, err := cd.getByteSizeDriverStatus(key)
			if err != nil {
				t.Errorf("%v should provide these this information: %v. Error: %v", driver, key, err)
			}

			if float64(byteSizeValue) <= 0 {
				t.Errorf("%v byte value should never be empty.", key)
			}
		}
	}
}

func TestIsContainerRunning(t *testing.T) {
	imageId := os.Getenv("DOCKER_IMAGE")

	if imageId != "" {
		cd := NewCheckDockerForTest(t)
		_, isRunning := cd.IsContainerRunning(imageId)

		if !isRunning {
			t.Errorf("Container for image: %v should be running.", imageId)
		}
	}
}

func TestCountGhostsByImageId(t *testing.T) {
	imageId := os.Getenv("DOCKER_IMAGE")

	if imageId != "" {
		cd := NewCheckDockerForTest(t)
		_, isGhost := cd.IsContainerAGhost(imageId)

		if isGhost {
			t.Errorf("Container for image: %v should not be a ghost.", imageId)
		}
	}
}

func TestCheckMetaSpace(t *testing.T) {
	cd := NewCheckDockerForTest(t)

	status := cd.CheckMetaSpace(cd.WarnMetaSpace, cd.CritMetaSpace)
	if status == nil {
		t.Error("NagiosStatus struct should never be nil.")
	}
}

func TestCheckDataSpace(t *testing.T) {
	cd := NewCheckDockerForTest(t)

	status := cd.CheckDataSpace(cd.WarnMetaSpace, cd.CritMetaSpace)
	if status == nil {
		t.Error("NagiosStatus struct should never be nil.")
	}
}

func TestCheckImageContainerIsInGoodShape(t *testing.T) {
	imageId := os.Getenv("DOCKER_IMAGE")

	if imageId != "" {
		cd := NewCheckDockerForTest(t)
		status := cd.CheckImageContainerIsInGoodShape(imageId)

		if status == nil {
			t.Error("NagiosStatus struct should never be nil.")
		}
		if status.Value != 0 {
			t.Errorf("Container of image: %v should be healthy.", imageId)
		}
	}
}
