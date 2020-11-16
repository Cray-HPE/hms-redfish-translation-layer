// Copyright 2019-2020 Hewlett Packard Enterprise Development LP

package backend_helpers

import (
	"context"

	log "github.com/sirupsen/logrus"
)

func (cs *MockBackendHelper) RunPeriodic(ctx context.Context, env map[string]interface{}) (err error) {
	return nil
}

// If an interface has some amount of prep work it needs to do before it is ready.
func (cs *MockBackendHelper) SetupBackendHelper(ctx context.Context, env map[string]interface{}) (err error) {
	err = cs.CertificateService.InitForXName("localhost")
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Failed to initialize Certificate Service for localhost")
		return
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
