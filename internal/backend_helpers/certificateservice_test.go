// Copyright 2020 Hewlett Packard Enterprise Development LP
package backend_helpers

import (
	"crypto/tls"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

// Please note that the following self certificate and key was generated for this
// unit test
const testCertificate = `-----BEGIN CERTIFICATE-----
MIIDJDCCAgwCCQCzN6lRBLvJgzANBgkqhkiG9w0BAQsFADBUMQswCQYDVQQGEwJV
UzESMBAGA1UECAwJTWlubmVzb3RhMRQwEgYDVQQHDAtCbG9vbWluZ3RvbjENMAsG
A1UECgwEVGVzdDEMMAoGA1UECwwDUlRTMB4XDTIwMDkyMzE2MzkwOVoXDTIxMDky
MzE2MzkwOVowVDELMAkGA1UEBhMCVVMxEjAQBgNVBAgMCU1pbm5lc290YTEUMBIG
A1UEBwwLQmxvb21pbmd0b24xDTALBgNVBAoMBFRlc3QxDDAKBgNVBAsMA1JUUzCC
ASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAL7mRgYmJ41XoqIpnRptXM81
LBUiCPo4+ZCwb+BR+XMkWLpyUP8IqEDFG0iruUz5Qam6HqXdeaeCAIo0gMP+BqPs
Xvk3ctZ3TZMNSwEw289ONC0PGL0zAEbE7b1HXj48Q62g61T7CMXtdoIV0JDOh2C9
Ssd/wo5mZLjIHRiatNnm2mXumztlSQPHD7I91cFoLaX9ckbeLIlDC4va0uGdp1fc
80IDgoWNBgqmpfvZVcWDlZ4K8hoPCZZpFr9PTHLRRFSLTxROxBA2efUkBI91OV0D
alGk3kCG5Rm26eCmQXSFWwRUnJJje3dGQ3t+SKNr3fzqrP22jml9rWpcD5A29KsC
AwEAATANBgkqhkiG9w0BAQsFAAOCAQEAircV8+zZj36CMoLmrE7HOo1MdP04Ju9M
J0ciHf7h3f1KmIl23UgQGo1vIOtyZmi8ZNAVXh3OVniUZQgcW3dMDExTdNpnLoCS
N1uohoRi/EOPuqeIzh2qb2A2QnvNz4NoLZ2bW7NS84NpEWiBuqDPO6fOD9nd5G4M
BV3S1+z0Pw2rJ4PlxLGYdu4chO0Pn3qofrEDM6KbYJ714SDUIuCA/sfCDCy+CV0O
piW27dw0fXEvWnxjzMXdTbhAPCeMG8pXHIg529SIUUts9Whhs3qquypRcInWAdDu
LIA5Hy5kDK4mSyNSh80dIKJxJNW4BHb+VrmG2lGNH6DsMBXOMaAJCQ==
-----END CERTIFICATE-----`

const testPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAvuZGBiYnjVeioimdGm1czzUsFSII+jj5kLBv4FH5cyRYunJQ
/wioQMUbSKu5TPlBqboepd15p4IAijSAw/4Go+xe+Tdy1ndNkw1LATDbz040LQ8Y
vTMARsTtvUdePjxDraDrVPsIxe12ghXQkM6HYL1Kx3/CjmZkuMgdGJq02ebaZe6b
O2VJA8cPsj3VwWgtpf1yRt4siUMLi9rS4Z2nV9zzQgOChY0GCqal+9lVxYOVngry
Gg8JlmkWv09MctFEVItPFE7EEDZ59SQEj3U5XQNqUaTeQIblGbbp4KZBdIVbBFSc
kmN7d0ZDe35Io2vd/Oqs/baOaX2talwPkDb0qwIDAQABAoIBAEhCU8kqEhifTnFB
fTUupk3Mm7YYHvvQKy9IieCIRvr9jBRvBxeySDXUJkK4tbhcNS2wxL8V+WGdhOTL
gN4vPoY4B68f/PkPSa7a/kQiIWH0AS35I+0h6/3dtvvJkvPNzfRgEBQnvadl/lC5
PyxA8N9+Z1rikltiiMek/9Z7YO+FmYZ6KADgZ+a2qh+ZUmNXjYMlJuZ9sz93eUmE
WtssGJp0j7UZEhYPDERFA6uW6kpDdW8r3spffRlPOZgKD1qZ4A5MHKuwD7zRMlao
aFOROSeF9CXlnrmQG/Sxj1AP1zJnxd1/5LCiTtqe+dqM1Zncr3wJG3589oFJ79Ml
cAeEr2ECgYEA886yW3LE5duI6kcfv7qUYCJm+xhwgw5DX9N6UGloIA8B30rJW8/W
abemZKR+XwG38U+cQPJVD4M+oahqvZ8xX1YbwFvQ+dIGILCJ5G7pzQWfgk3Q2oX1
23lpW2FV89eT5dU3/mmGG/kLC63axy51QzDb7ti8G5b5Wkp7eadTWRECgYEAyHI7
eGBc2O8wlnpdirRNNHIMO66NGTMQLKohFMQcWczVEfzlGtyLezxwdiv0gE2zJWN7
OxXZlMTOJiGD66UvCCa2MipnEnDJcHp5LMokh1cZpj0qqZMDUpP0j0u8AcLObhTP
N+WHZ7f2F4nfZkSHt11kyF12pVL+ALNSbitMkfsCgYBdIklZy6bRk6JitFa5fAGw
E5Q5OSXJuoocMgHYc9uV24XAkaYHz4Y9ji0e5wNrMZHduaab3LaHnYAwatCTrRtE
KvWg7rIOrJ8wn5+dRo1Dh3FeanFs+J1pgKCxiqY15tUVh/TC1/al+uWwSXJ4ghPD
Xge13s9EztBkIG24lCWvsQKBgAPEofmRVi190ZwCkN+apBjoS/KTRXPD0foE+Lo7
NY06nIbKCkSHANhAOpz+FoqS61s4k4h40K5LRNTSrHgxksDEeYhX47glBqRmqQB+
jFE/AexuGe82JEnZHi/TbKVb1CWdnoeeeP0qKCYpIVn6z9JSnyJlH2XcOYop1NLd
XYMhAoGBAILytFdgnrYrph+voh5S0YMMZNMIXkXVA0iBEf/6Zv8NfqqvKiqMAu0d
iYNN5ugsByuYJcZ9kyAqbJfqyQqklcNdHQY6Ysj4L1MuamjPELT0bzzjvFU6EwZy
4YbTXiSnY5ZNw/aERRYfki7X7/ixhfLJkVXdFdnm6aZE9Soub5eC
-----END RSA PRIVATE KEY-----`

type CertificateService_TS struct {
	suite.Suite

	cs *CertificateService
}

func (suite *CertificateService_TS) SetupTest() {
	suite.cs = &CertificateService{
		Certificates: map[string]*CertificateTuple{},
	}
}

func (suite *CertificateService_TS) Test_ParseCertificatePairFromString_HappyPath() {
	certificateString := fmt.Sprintf("%s\n%s", testCertificate, testPrivateKey)

	certPair, err := suite.cs.ParseCertificatePairFromString(certificateString)
	suite.NoError(err)

	suite.Equal(certPair.Certificate, testCertificate)
	suite.Equal(certPair.PrivateKey, testPrivateKey)
}

func (suite *CertificateService_TS) Test_ParseCertificatePairFromString_SadPath() {
	certificateStrings := []string{
		"",              // No Data
		testCertificate, // Only Certificate is provided
		testPrivateKey,  // Only private key is provided
		"foobar",        // Invalid Data
	}

	for _, certificateString := range certificateStrings {
		_, err := suite.cs.ParseCertificatePairFromString(certificateString)
		suite.Error(err)

	}

}

func (suite *CertificateService_TS) Test_LoadCustomCertificate_HappyPath() {
	// Build certificate pair
	certificateString := fmt.Sprintf("%s\n%s", testCertificate, testPrivateKey)
	certPair, err := suite.cs.ParseCertificatePairFromString(certificateString)
	suite.NoError(err)

	// Load Certificate
	err = suite.cs.LoadCertificateFromPair("x0m0", certPair)
	suite.NoError(err)

	// Lookup certificate pair
	certTuple := suite.cs.GetCertificate("x0m0")
	suite.NotNil(certTuple)
	suite.ElementsMatch([]string{"RTS"}, certTuple.X509.Issuer.OrganizationalUnit)
	suite.ElementsMatch([]string{"Test"}, certTuple.X509.Issuer.Organization)
	suite.ElementsMatch([]string{"Bloomington"}, certTuple.X509.Issuer.Locality)
	suite.ElementsMatch([]string{"Minnesota"}, certTuple.X509.Issuer.Province)
	suite.ElementsMatch([]string{"US"}, certTuple.X509.Issuer.Country)

	// Test ReturnCert, this determines what the HTTPS Server will use as the certificate
	tlsClientInfo := &tls.ClientHelloInfo{
		ServerName: "x0m0",
	}
	tlsCert, err := suite.cs.ReturnCert(tlsClientInfo)
	suite.NoError(err)
	suite.Equal(certTuple.TLS, tlsCert)
}

func Test_CertificateService(t *testing.T) {
	//This setups the production routs and handler
	logrus.SetLevel(logrus.TraceLevel)
	suite.Run(t, new(JAWSTelemetryHelper_TS))
}
