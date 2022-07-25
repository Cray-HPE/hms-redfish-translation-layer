// MIT License
//
// (C) Copyright [2019-2021] Hewlett Packard Enterprise Development LP
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

package backend_helpers

import (
	"context"
	"errors"
	"os"

	log "github.com/sirupsen/logrus"
)

func (cs *MockBackendHelper) RunPeriodic(ctx context.Context, env map[string]interface{}) (err error) {
	return nil
}

// If an interface has some amount of prep work it needs to do before it is ready.
func (cs *MockBackendHelper) SetupBackendHelper(ctx context.Context, env map[string]interface{}) error {
	xname := "localhost"

	// Setup root user
	password, passwordExists := os.LookupEnv("MOCKBACKEND_ROOT_PASSWORD")
	if !passwordExists {
		return errors.New("environment variable MOCKBACKEND_ROOT_PASSWORD is not defined")

	}
	err := cs.RedisHelper.initAccount("root", "Administrator", password)
	if err != nil {
		return err
	}

	// Setup Manager collection
	err = cs.RedisHelper.initManagerCollection(xname)
	if err != nil {
		log.WithError(err).Error("Failed to initialize Manager Collection for device")
		return err
	}

	err = cs.RedisHelper.initManager(xname, GenericBmcID, "Mock")
	if err != nil {
		log.WithError(err).Error("Failed to initialize Manager for device")
		return err
	}

	// Certificate Service
	err = cs.CertificateService.SetupCertificateService(GenericBmcID, GenericCertificateID)
	if err != nil {
		log.WithError(err).Error("Failed to setup Certificate service")
		return err
	}

	err = cs.CertificateService.InitForXName(xname)
	if err != nil {
		log.WithError(err).Error("Failed to initialize Certificate Service for localhost")
		return err
	}
	return nil
}

func (cs *MockBackendHelper) GetEnvForXname(xname string) (env map[string]string, err error) {
	return map[string]string{}, nil
}

// Called for every request a backend might be able to service.
func (cs *MockBackendHelper) RunBackendHelper(ctx context.Context, key string, args []string, env map[string]string) (value string, err error) {
	// Do Nothing
	return "", MockNoOpError
}
