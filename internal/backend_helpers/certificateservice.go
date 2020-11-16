// Copyright 2020 Hewlett Packard Enterprise Development LP
package backend_helpers

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfdispatcher/certificate_store"
)

const CertificateServiceKeyspace = ServiceRootKeyspace + "/CertificateService"
const ManagersKeyspace = ServiceRootKeyspace + "/Managers"

type CertificateTuple struct {
	PublicCertificate string
	TLS               *tls.Certificate
	X509              *x509.Certificate
}

func (cs *CertificateService) initCertificateService(xname, bmcID, certificateID string) (err error) {
	// The service root is special, so we don't want to prepend the xname at the beginning
	// of the key
	keyspace, err := cs.RedisHelper.addMemberToSet(ServiceRootKeyspace, "CertificateService")
	if err != nil {
		return err
	}

	// Here we want to prepend the xname, so this xname gets its own certificate service
	keyspace = filepath.Join(xname, keyspace)

	cs.RedisHelper.addMemberToSet(keyspace, "@odata.id")
	cs.RedisHelper.addMemberToSet(keyspace, "@odata.type")

	cs.RedisHelper.setValueForPropertyInKeyspace(keyspace, "Id", "CertificateService")
	cs.RedisHelper.setValueForPropertyInKeyspace(keyspace, "Name", "Certificate Service")

	// Certificate Replacement Action
	actionsKeyspace, addErr := cs.RedisHelper.addMemberToSet(keyspace, "Actions")
	if addErr != nil {
		err = fmt.Errorf("unable to add Actions to Certificate Service")
		return
	}
	actionName := "CertificateService.ReplaceCertificate"
	replaceCertificateKeyspace, addErr := cs.RedisHelper.addMemberToSet(actionsKeyspace, "#"+actionName)
	if addErr != nil {
		err = fmt.Errorf("unable to add reaplace certificate action to Certificate Service")
		return
	}

	replaceCertificateTargetURI := filepath.Join(actionsKeyspace, actionName)
	cs.RedisHelper.setValueForPropertyInKeyspace(replaceCertificateKeyspace, "Target", replaceCertificateTargetURI)

	allowableValuesKeyspace, addErr := cs.RedisHelper.addMemberToSet(replaceCertificateKeyspace,
		"CertificateType@Redfish.AllowableValues")
	if addErr != nil {
		err = fmt.Errorf("unable to add AllowableValues to replace certificate action")
		return
	}
	cs.RedisHelper.Redis.RPush(allowableValuesKeyspace, "PEM")

	return nil
}

func (cs *CertificateService) initCertificateLocations(xname, bmcID, certificateID string) (err error) {
	keyspace, err := cs.RedisHelper.addMemberToSet(filepath.Join(xname, CertificateServiceKeyspace), "CertificateLocations")
	if err != nil {
		return err
	}

	cs.RedisHelper.addMemberToSet(keyspace, "@odata.id")
	cs.RedisHelper.addMemberToSet(keyspace, "@odata.type")

	cs.RedisHelper.setValueForPropertyInKeyspace(keyspace, "Id", "CertificateLocations")
	cs.RedisHelper.setValueForPropertyInKeyspace(keyspace, "Name", "Certificate Locations")

	linksKeyspace, err := cs.RedisHelper.addMemberToSet(keyspace, "Links")
	if err != nil {
		return err
	}

	// Might it make sense to create this when Initializing the manager?
	certificateLinkKeyspace, err := cs.RedisHelper.addMemberToArray(linksKeyspace, "Certificates")
	if err != nil {
		return err
	}

	managerHTTPSCertLink := ManagersKeyspace + "/" + bmcID + "/NetworkProtocol/HTTPS/Certificates/" + certificateID
	cs.RedisHelper.setValueForPropertyInKeyspace(certificateLinkKeyspace, "@odata.id", managerHTTPSCertLink)

	return nil
}

func (cs *CertificateService) initManagerNetworkProtocol(xname, bmcID, certificateID string) (err error) {
	// Assume that there is only 1 manager
	managerKeyspace := filepath.Join(xname, ManagersKeyspace, bmcID)

	keyspace, err := cs.RedisHelper.addMemberToSet(managerKeyspace, "NetworkProtocol")
	if err != nil {
		return err
	}

	cs.RedisHelper.addMemberToSet(keyspace, "@odata.id")
	cs.RedisHelper.addMemberToSet(keyspace, "@odata.type")

	// Add navigation link for HTTPS Certificates
	httpsKeyspace, err := cs.RedisHelper.addMemberToSet(keyspace, "HTTPS")
	if err != nil {
		return nil
	}

	_ = httpsKeyspace

	return nil
}

func (cs *CertificateService) initManagerHTTPSCertificateCollection(xname, bmcID, certificateID string) (err error) {
	// Assume that there is only 1 manager
	rootKeyspace := filepath.Join(xname, ManagersKeyspace, bmcID, "NetworkProtocol", "HTTPS")

	keyspace, err := cs.RedisHelper.addMemberToSet(rootKeyspace, "Certificates")
	if err != nil {
		return nil
	}

	cs.RedisHelper.addMemberToSet(keyspace, "@odata.id")
	cs.RedisHelper.addMemberToSet(keyspace, "@odata.type")
	cs.RedisHelper.setValueForPropertyInKeyspace(keyspace, "Description", "A collection of all Certificates instances for this manager")
	cs.RedisHelper.setValueForPropertyInKeyspace(keyspace, "Name", "Certificates Collection")

	// Initialize an empty collection members array
	cs.RedisHelper.addMemberToSet(keyspace, "Members")

	return nil
}

func (cs *CertificateService) initManagerHTTPSCertificate(xname, bmcID, certificateID string) (err error) {
	collectionKeyspace := filepath.Join(xname, ManagersKeyspace, bmcID, "/NetworkProtocol/HTTPS/Certificates")
	keyspace := filepath.Join(collectionKeyspace, certificateID)

	cs.RedisHelper.addMemberToSet(keyspace, "@odata.id")
	cs.RedisHelper.addMemberToSet(keyspace, "@odata.type")

	cs.RedisHelper.setValueForPropertyInKeyspace(keyspace, "Id", certificateID)
	cs.RedisHelper.setValueForPropertyInKeyspace(keyspace, "Name", "HTTPS Certificate for RTS")

	cs.RedisHelper.addMemberToSet(keyspace, "CertificateString")
	cs.RedisHelper.addMemberToSet(keyspace, "CertificateType")
	cs.RedisHelper.addMemberToSet(keyspace, "ValidNotAfter")
	cs.RedisHelper.addMemberToSet(keyspace, "ValidNotBefore")

	// Issuer
	issuerKeyspace, err := cs.RedisHelper.addMemberToSet(keyspace, "Issuer")
	if err != nil {
		return nil
	}

	cs.RedisHelper.addMemberToSet(issuerKeyspace, "City")
	cs.RedisHelper.addMemberToSet(issuerKeyspace, "CommonName")
	cs.RedisHelper.addMemberToSet(issuerKeyspace, "Country")
	cs.RedisHelper.addMemberToSet(issuerKeyspace, "Organization")
	cs.RedisHelper.addMemberToSet(issuerKeyspace, "OrganizationalUnit")
	cs.RedisHelper.addMemberToSet(issuerKeyspace, "State")

	// Subject
	subjectKeyspace, err := cs.RedisHelper.addMemberToSet(keyspace, "Subject")
	if err != nil {
		return nil
	}

	cs.RedisHelper.addMemberToSet(subjectKeyspace, "City")
	cs.RedisHelper.addMemberToSet(subjectKeyspace, "CommonName")
	cs.RedisHelper.addMemberToSet(subjectKeyspace, "Country")
	cs.RedisHelper.addMemberToSet(subjectKeyspace, "Organization")
	cs.RedisHelper.addMemberToSet(subjectKeyspace, "OrganizationalUnit")
	cs.RedisHelper.addMemberToSet(subjectKeyspace, "State")

	// Add Cert to collection
	memberKeyspace, err := cs.RedisHelper.addMemberToArray(collectionKeyspace, "Members")
	if err != nil {
		return
	}
	cs.RedisHelper.setValueForPropertyInKeyspace(memberKeyspace, "@odata.id", keyspace)

	return
}

// SetupCertificateService is intended to be called by Device Backend helpers so the setup the certificate service
// with the correct BMC manager and certificate IDs,
func (cs *CertificateService) SetupCertificateService(bmcID, certificateID string) (err error) {
	cs.BmcID = bmcID
	cs.CertificateID = certificateID

	return nil
}

// InitForXName is intended to be called by Device Backend helpers that are
// in charge of creating the xname personans that RTS takes on.
func (cs *CertificateService) InitForXName(xname string) (err error) {
	logCtx := logrus.WithField("xname", xname)

	// Initialize Redis
	type initFunctionType func(xname, bmcID, certificateID string) error
	initFunctions := []initFunctionType{
		cs.initCertificateService,
		cs.initCertificateLocations,
		cs.initManagerNetworkProtocol,
		cs.initManagerHTTPSCertificateCollection,
		cs.initManagerHTTPSCertificate,
	}

	for _, initFunction := range initFunctions {
		name := runtime.FuncForPC(reflect.ValueOf(initFunction).Pointer()).Name()
		logCtx.WithField("initFunction", name)

		err = initFunction(xname, cs.BmcID, cs.CertificateID)
		if err != nil {
			logCtx.WithError(err).Error("Initialization function failed")

			return
		}

		logCtx.Debug("Initialization function succeeded")
	}

	if cs.CertificateStore != nil {
		// Check vault for the devices certificate pair
		pair, err := cs.CertificateStore.GetCertificatePair(xname)
		if err == certificate_store.ErrNotFound {
			// Vault does not have a certificate for us. The default Certificate will be used
		} else if err != nil {
			logCtx.WithError(err).Error("Unable to get certificate from vault")
			return err
		} else {
			// Add the certificate
			err = cs.LoadCertificateFromPair(xname, pair)
			if err != nil {
				logCtx.WithError(err).Error("Unable to get load certificate pair from vault")
				return err
			}
		}
	}

	// Keep track of devices that we are managing
	cs.KnownDevices[xname] = true

	return nil
}

func (cs *CertificateService) populateCertificate(pl redis.Pipeliner, xname, bmcID, certID string) error {
	keyspace := filepath.Join(xname, ManagersKeyspace, bmcID, "NetworkProtocol", "HTTPS", "Certificates", certID)

	// Lookup the certificate for this device
	cert := cs.GetCertificate(xname)

	pl.Set(filepath.Join(keyspace, "CertificateString"), cert.PublicCertificate, 0)
	pl.Set(filepath.Join(keyspace, "CertificateType"), "PEM", 0)
	pl.Set(filepath.Join(keyspace, "ValidNotAfter"), cert.X509.NotAfter.Format(time.RFC3339), 0)
	pl.Set(filepath.Join(keyspace, "ValidNotBefore"), cert.X509.NotBefore.Format(time.RFC3339), 0)

	// Issuer
	issuerKeyspace := filepath.Join(keyspace, "Issuer")
	issuer := cert.X509.Issuer

	pl.Set(filepath.Join(issuerKeyspace, "City"), strings.Join(issuer.Locality, " "), 0)
	pl.Set(filepath.Join(issuerKeyspace, "CommonName"), issuer.CommonName, 0)
	pl.Set(filepath.Join(issuerKeyspace, "Country"), strings.Join(issuer.Country, " "), 0)
	pl.Set(filepath.Join(issuerKeyspace, "Organization"), strings.Join(issuer.Organization, " "), 0)
	pl.Set(filepath.Join(issuerKeyspace, "OrganizationalUnit"), strings.Join(issuer.OrganizationalUnit, " "), 0)
	pl.Set(filepath.Join(issuerKeyspace, "State"), strings.Join(issuer.Province, " "), 0)

	// Subject
	subjectKeyspace := filepath.Join(keyspace, "Subject")
	subject := cert.X509.Subject

	pl.Set(filepath.Join(subjectKeyspace, "City"), strings.Join(subject.Locality, " "), 0)
	pl.Set(filepath.Join(subjectKeyspace, "CommonName"), subject.CommonName, 0)
	pl.Set(filepath.Join(subjectKeyspace, "Country"), strings.Join(subject.Country, " "), 0)
	pl.Set(filepath.Join(subjectKeyspace, "Organization"), strings.Join(subject.Organization, " "), 0)
	pl.Set(filepath.Join(subjectKeyspace, "OrganizationalUnit"), strings.Join(subject.OrganizationalUnit, " "), 0)
	pl.Set(filepath.Join(subjectKeyspace, "State"), strings.Join(subject.Province, " "), 0)

	return nil

}

// If an interface has some amount of prep work it needs to do before it is ready.
func (cs *CertificateService) SetupBackendHelper(ctx context.Context, env map[string]interface{}) (err error) {
	// It is the job of the other backend helpers to setup the Certificate service for the devices that they control
	// However there are some things that the Certificate Service needs to setup for itself
	cs.KnownDevices = map[string]bool{}
	cs.Certificates = map[string]*CertificateTuple{}

	// Time to setup the connection to Vault and the Certificate Store
	certificateVaultKeypath, ok := os.LookupEnv("CERTIFICATE_VAULT_KEYPATH")
	if !ok || certificateVaultKeypath == "" {
		log.Warn("Environment variable CERTIFICATE_VAULT_KEYPATH was not set or is empty")
		log.Warn("The Certificate Service will not use vault to store certificates")
		return
	}

	secureStorage := connectToVault()
	if secureStorage == nil {
		log.Fatal("Unable to connect to Vault")
	}

	cs.CertificateStore = certificate_store.NewCertificateStore(certificateVaultKeypath, secureStorage)

	return nil
}

func (cs *CertificateService) GetEnvForXname(xname string) (env map[string]string, err error) {
	return map[string]string{"RTS_XNAME": xname}, nil
}

// Called for every request a backend might be able to service.
func (cs *CertificateService) RunBackendHelper(ctx context.Context, key string, args []string, env map[string]string) (value string, err error) {
	xname, ok := env["RTS_XNAME"]
	if !ok {
		err = errors.New("RTS_XNAME not in environment variables")
		log.Error(err.Error())
		return
	}

	// Check to make sure we actually know about this device.
	if _, deviceIsKnown := cs.KnownDevices[xname]; !deviceIsKnown {
		err = fmt.Errorf("unknown xname: %s", xname)
		log.Error(err.Error())
		return
	}

	// Do some sanity checks to make sure we're getting what we expect.
	// Every key that comes in could be xname prefixed, use a regular expression to capture that.
	keyRegex := regexp.MustCompile(`(x[0-9]{1,4}c[0-7]s[0-9]+b[0-9]+)*(` + ServiceRootKeyspace + `\S+$)`)
	keyMatches := keyRegex.FindStringSubmatch(key)

	var fullKey string
	switch len(keyMatches) {
	case 2:
		// Not xname prefixed
		fullKey = keyMatches[1]
	case 3:
		// Xname prefixed
		fullKey = keyMatches[2]
	default:
		err = errors.New("key format not what expected")
		log.WithField("key", key).Error("Unable to run regex expression on key")
		return
	}

	logCtx := logrus.WithFields(logrus.Fields{
		"key":     key,
		"fullKey": fullKey,
		"xname":   xname,
	})

	// Keys that this backend helper can respond to:
	// /redfish/v1/Managers/BMC/NetworkProtocol/HTTPS/Certificates/1/*
	// /redfish/v1/CertificateService/Actions/CertificateService.ReplaceCertificate

	managerHTTPSCertificateKeyspace := filepath.Join(ManagersKeyspace, cs.BmcID, "NetworkProtocol/HTTPS/Certificates", cs.CertificateID)

	if strings.HasPrefix(fullKey, managerHTTPSCertificateKeyspace) {
		pl := cs.RedisHelper.Redis.Pipeline()
		cs.populateCertificate(pl, xname, cs.BmcID, cs.CertificateID)
		if _, err := pl.Exec(); err != nil {
			logrus.WithError(err).Error("Failed to run exec")
			return "", err
		}
	} else if fullKey == "/redfish/v1/CertificateService/Actions/CertificateService.ReplaceCertificate" {
		certificateURI, ok := env["CertificateUri"]
		if !ok {
			err = fmt.Errorf("CertificateUri was not provided in the certificate replace request")
			logCtx.Error(err)
			return
		}

		certificateType, ok := env["CertificateType"]
		if !ok {
			err = fmt.Errorf("CertificateType was not provided in the certificate replace request")
			logCtx.Error(err)
			return
		}

		// Right now we only accept PEM formated certificates
		if certificateType != "PEM" {
			err = fmt.Errorf("Unsupported CertificateType provided in the certificate replace request")
			logCtx.Error(err)
			return
		}

		certificateString, ok := env["CertificateString"]
		if !ok {
			err = fmt.Errorf("CertificateString was not provided in the certificate replace request")
			logCtx.Error(err)
			return
		}

		// Right now we can only update the certificate for the manager (RTS)
		if certificateURI != managerHTTPSCertificateKeyspace {
			err = fmt.Errorf("Unknown Certificate URI")
			logCtx.Error(err)
			return
		}

		// Handler Certificate Replacement
		var certPair certificate_store.CertificatePair
		certPair, err = cs.ParseCertificatePairFromString(certificateString)
		if err != nil {
			logCtx.WithError(err).Error("Failed to parse certificate string")
			return
		}

		logCtx.Debug("Adding Cert")
		if err = cs.LoadCertificateFromPair(xname, certPair); err != nil {
			logCtx.WithError(err).Error("Failed to parse certificate string")
			return
		}

		// Update Vault wit the new certificate
		logCtx.Debug("Storing Certificate in Vault")
		if cs.CertificateStore != nil {
			err = cs.CertificateStore.StoreCertificatePair(xname, certPair)
			if err != nil {
				return "", fmt.Errorf("Unable to store certifcate pair in Vault: %w", err)
			}
		}

		// Update Redis with the new certificate information
		logCtx.Debug("Updating Redis")
		pl := cs.RedisHelper.Redis.Pipeline()
		cs.populateCertificate(pl, xname, cs.BmcID, cs.CertificateID)
		if _, err := pl.Exec(); err != nil {
			logrus.WithError(err).Error("Failed to run exec")
			return "", err
		}

		return

	} else {
		// This key does not apply to this backend helper
		return "", ErrBackendContinue
	}

	// Finally query redis for the key
	return cs.RedisHelper.Redis.Get(key).Result()
}

// If an interface has some refreshing it needs to do at some interval.
func (cs *CertificateService) RunPeriodic(ctx context.Context, env map[string]interface{}) (err error) {
	// Nothing needs to be done on an periodic basis
	return nil
}

// Get the certificate for the provided xname. If it does not have one, return the default cert
func (cs *CertificateService) GetCertificate(xname string) *CertificateTuple {
	cs.CertificatesLock.Lock()
	defer cs.CertificatesLock.Unlock()

	cert, ok := cs.Certificates[xname]
	if !ok {
		cert = cs.DefaultCertificate
	}

	return cert
}

func (cs *CertificateService) ParseCertificatePairFromString(certificateString string) (certificate_store.CertificatePair, error) {
	var certPEMBlock *pem.Block
	var keyPEMBlock *pem.Block

	// It is expected that both the certificate and private key are present
	// in the PEM formated string. It does not matter for the order of the
	// pem blocks.
	//
	// Example:
	// -----BEGIN CERTIFICATE-----
	// MIIDJDCC.........
	// -----END CERTIFICATE-----
	// -----BEGIN RSA PRIVATE KEY-----
	// MIIEowIB.........
	// -----END RSA PRIVATE KEY-----

	// Parse the certificate string to extract the certificate and private key
	certificateRaw := []byte(certificateString)
	for len(certificateRaw) != 0 {
		block, next := pem.Decode(certificateRaw)

		// Note, the pem.Decode will ignore data that does not conform to the format.
		// If there are no valid PEM blocks the next value will be equal to what was put in
		if bytes.Equal(certificateRaw, next) {
			break
		}
		// If the data that is decode has no valid PEM blocks then a nil block will be
		// returned.
		if block == nil {
			continue
		}

		if block.Type == "CERTIFICATE" {
			certPEMBlock = block
		} else if block.Type == "RSA PRIVATE KEY" {
			keyPEMBlock = block
		}

		certificateRaw = next
	}

	if certPEMBlock == nil {
		return certificate_store.CertificatePair{}, fmt.Errorf("certificate string is missing PEM formated certificate")
	}

	if keyPEMBlock == nil {
		return certificate_store.CertificatePair{}, fmt.Errorf("certificate string is missing PEM formated private key")
	}

	// The TLS package needs the PEM encoded certificate, not the decoded one
	certPEM := pem.EncodeToMemory(certPEMBlock)
	keyPEM := pem.EncodeToMemory(keyPEMBlock)

	pair := certificate_store.CertificatePair{
		Certificate: string(certPEM),
		PrivateKey:  string(keyPEM),
	}

	return pair, nil
}

func (cs *CertificateService) LoadCertificateFromPair(xname string, pair certificate_store.CertificatePair) error {
	certPEMBlock, _ := pem.Decode([]byte(pair.Certificate))
	keyPEMBlock, _ := pem.Decode([]byte(pair.PrivateKey))

	// Load the TLS Cert and Key
	// The TLS package needs the PEM encoded certificate, not the decoded one
	certPEM := pem.EncodeToMemory(certPEMBlock)
	keyPEM := pem.EncodeToMemory(keyPEMBlock)
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("Failed to load X509 Keypair: %w", err)
	}

	// Load the X509 information
	x509Cert, err := x509.ParseCertificate(certPEMBlock.Bytes)
	if err != nil {
		return fmt.Errorf("Failed to parse certificate: %w", err)
	}

	// Build up out certificate tuple
	certTuple := CertificateTuple{
		TLS:               &tlsCert,
		X509:              x509Cert,
		PublicCertificate: pair.Certificate,
	}

	// Update our certificate map
	cs.CertificatesLock.Lock()
	cs.Certificates[xname] = &certTuple
	cs.CertificatesLock.Unlock()

	return nil
}

func (cs *CertificateService) LoadDefaultCert(certFile, keyFile string) error {
	// Load TLS Cert and Key
	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	// Load X509 information
	certRaw, err := ioutil.ReadFile(certFile)
	if err != nil {
		return err
	}

	block, _ := pem.Decode(certRaw)

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	cs.DefaultCertificate = &CertificateTuple{
		TLS:               &tlsCert,
		X509:              x509Cert,
		PublicCertificate: string(certRaw),
	}
	return nil
}

func (cs *CertificateService) ReturnCert(clientInfo *tls.ClientHelloInfo) (*tls.Certificate, error) {
	// Strip off '-rts' prefix on the servername if present.
	xname := strings.TrimSuffix(clientInfo.ServerName, "-rts")

	logCtx := logrus.WithFields(log.Fields{
		"ServerName": clientInfo.ServerName,
		"xname":      xname,
	})

	cs.CertificatesLock.Lock()
	defer cs.CertificatesLock.Unlock()

	logCtx.Debug("Looking up certificate")
	if certificate, ok := cs.Certificates[xname]; ok {
		logCtx.Debug("Found certificate")
		return certificate.TLS, nil
	}

	logCtx.Debug("Using default certificate")
	return cs.DefaultCertificate.TLS, nil
}
