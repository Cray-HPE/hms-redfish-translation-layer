// MIT License
//
// (C) Copyright [2020-2023] Hewlett Packard Enterprise Development LP
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
	"sync"
	"time"

	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/certificate_store"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/pdu_credential_store"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/rts_credential_store"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/slsapi"
	"github.com/fsnotify/fsnotify"
	"google.golang.org/api/compute/v1"
)

// Redfish CertificateService implementation
type CertificateService struct {
	RedisHelper RedisHelper

	BmcID         string
	CertificateID string

	CertificateStore *certificate_store.CertificateStore

	DefaultCertificate        *CertificateTuple
	defaultCertificateWatcher *fsnotify.Watcher
	Certificates              map[string]*CertificateTuple
	CertificatesLock          sync.Mutex

	KnownDevices map[string]bool
}

// This is just a generic no-op interface
type MockBackendHelper struct {
	CertificateService *CertificateService

	RedisHelper RedisHelper
}

// ServerTech iPDU API called JAWS.
type JAWSBackedHelper struct {
	PDUCredentialStore *pdu_credential_store.PDUCredentialStore
	RTSCredentialStore *rts_credential_store.RTSCredentialStore

	CertificateService *CertificateService

	RedisHelper  RedisHelper
	KnownDevices map[string]pdu_credential_store.Device

	PollingEnabled  bool
	PollingInterval time.Duration
	PollingWorkers  int
	CurrentPolling  *XNameSet

	PollingTemperatureInterval time.Duration
	PollingHumidityInterval    time.Duration
}

// Google Cloud
type GCloudHelper struct {
	// Cached information about Redis
	RedisHelper RedisHelper

	// Redfish Certificate Service
	CertificateService *CertificateService

	// The GCP Project ID of the VShasta project.  Set by
	// GCLOUD_PROJECT if present, otherwise set automatically on
	// VShasta or an error on standalone.
	project string

	// The namespace in which to create XNAME Redfish Endpoint
	// services, set by the XNAME_NAMESPACE environment variable
	// if present, otherwise defaults to 'services'.
	namespace string

	// Cached connection to GCE API used to make GCE calls.
	computeService *compute.Service

	// Xname to instance mapping.
	KnownInstances map[string]*compute.Instance

	// XName endpoints awaiting readiness check (retry counter per
	// XName)
	waitReady map[string]int

	// XName endpoints awaiting HSM publication (retry counter per
	// XName)
	informHSM map[string]int

	RTSCredentialStore *rts_credential_store.RTSCredentialStore
}

type RFSimulatorHelper struct {
	RedisHelper        RedisHelper
	CertificateService *CertificateService
	KnownInstances     map[string]interface{}
}

// Management Switch SNMP Interface
type SNMPSwitchHelper struct {
	RTSCredentialStore *rts_credential_store.RTSCredentialStore

	CertificateService *CertificateService

	SLS *slsapi.SLS

	RedisHelper  RedisHelper
	KnownDevices map[string]SNMPDevice

	deviceMux sync.Mutex
}

type BackendHelper interface {
	// If an interface has some refreshing it needs to do at some interval.
	RunPeriodic(ctx context.Context, env map[string]interface{}) (err error)

	// If an interface has some amount of prep work it needs to do before it is ready.
	SetupBackendHelper(ctx context.Context, env map[string]interface{}) (err error)

	// Get Environment for backend handler invocations
	GetEnvForXname(xname string) (env map[string]string, err error)

	// Called for every request a backend might be able to service.
	RunBackendHelper(ctx context.Context, key string, args []string, env map[string]string) (value string, err error)
}

// Generic error used for discerning whether the RunBackendHelper function actually did something.
var MockNoOpError = errors.New("RunBackendHelper executed with Mock interface")

// The current backend helper does not apply to the current backend helper
var ErrBackendContinue = errors.New("continue onto the next backend helper")
