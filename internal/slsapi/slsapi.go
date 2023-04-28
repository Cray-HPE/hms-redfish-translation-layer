// MIT License
//
// (C) Copyright [2023] Hewlett Packard Enterprise Development LP
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

package slsapi

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	base "github.com/Cray-HPE/hms-base"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/sirupsen/logrus"
)

const DefaultSlsUrl string = "http://cray-sls/"

type SLS struct {
	Url    *url.URL
	Client *retryablehttp.Client
}

type slsReady struct {
	Ready  bool   `json:"Ready"`
	Reason string `json:"Reason,omitempty"`
	Code   int    `json:"Code,omitempty"`
}

type ManagementSwitch struct {
	Parent          string                        `json:"Parent"`
	Children        []string                      `json:"Children,omitempty"`
	Xname           string                        `json:"Xname"`
	Type            sls_common.HMSStringType      `json:"Type"`
	Class           sls_common.CabinetType        `json:"Class"`
	TypeString      base.HMSType                  `json:"TypeString"`
	LastUpdated     int64                         `json:"LastUpdated,omitempty"`
	LastUpdatedTime string                        `json:"LastUpdatedTime,omitempty"`
	ExtraProperties sls_common.ComptypeMgmtSwitch `json:"ExtraProperties,omitempty"`
}

var serviceName string

// Allocate and initialize new SLS struct.
func NewSLS(slsUrl string, httpClient *retryablehttp.Client, svcName string) *SLS {
	var err error
	serviceName = svcName
	sls := new(SLS)
	if sls.Url, err = url.Parse(slsUrl); err != nil {
		sls.Url, _ = url.Parse(DefaultSlsUrl)
	} else {
		// Default to using http if not specified
		if len(sls.Url.Scheme) == 0 {
			sls.Url.Scheme = "http"
		}
	}

	// Create an httpClient if one was not given
	if httpClient == nil {
		sls.Client = retryablehttp.NewClient()
		sls.Client.HTTPClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		sls.Client.RetryMax = 5
		sls.Client.HTTPClient.Timeout = time.Second * 40
		//turn off the http client loggin!
		tmpLogger := logrus.New()
		tmpLogger.SetLevel(logrus.PanicLevel)
		sls.Client.Logger = tmpLogger
	} else {
		sls.Client = httpClient
	}
	return sls
}

// Ping SLS to see if it is ready.
func (sls *SLS) IsReady() (bool, error) {
	var ready slsReady
	if sls.Url == nil {
		return false, fmt.Errorf("SLS struct has no URL")
	}
	uri := sls.Url.String() + "/ready"
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return false, err
	}
	body, err := sls.doRequest(req)
	if err != nil {
		return false, err
	}

	err = json.Unmarshal(body, &ready)
	if err != nil {
		return false, err
	}

	if !ready.Ready {
		err = fmt.Errorf("%d - %s", ready.Code, ready.Reason)
	}
	return ready.Ready, err
}

// Query SLS for all of the management switches it has.
func (sls *SLS) GetAllSwitchInfo() (switchHardware []ManagementSwitch, err error) {
	// Validate inputs
	if sls.Url == nil {
		err = fmt.Errorf("SLS struct has no URL")
		return
	}

	// Construct a GET to '/search/hardware?type=comptype_mgmt_switch'
	// for SLS to get all of the management switches in SLS.
	uri := sls.Url.String() + "/search/hardware?type=" + sls_common.MgmtSwitch.String()
	req, reqErr := http.NewRequest(http.MethodGet, uri, nil)
	if reqErr != nil {
		err = reqErr
		return
	}
	body, reqErr := sls.doRequest(req)
	if reqErr != nil {
		err = reqErr
		return
	}

	// SLS returns null if the component was not found
	if body == nil {
		return
	}

	reqErr = json.Unmarshal(body, &switchHardware)
	if reqErr != nil {
		err = reqErr
	}
	return
}

// doRequest sends a HTTP request to SLS
func (sls *SLS) doRequest(req *http.Request) ([]byte, error) {
	// Error if there is no client defined
	if sls.Client == nil {
		return nil, fmt.Errorf("SLS struct has no HTTP Client")
	}

	// Send the request
	base.SetHTTPUserAgent(req, serviceName)
	newRequest, err := retryablehttp.FromRequest(req)
	if err != nil {
		return nil, err
	}
	newRequest.Header.Set("Content-Type", "application/json")

	rsp, err := sls.Client.Do(newRequest)
	if err != nil {
		return nil, err
	}

	// Read the response
	defer rsp.Body.Close()
	body, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, err
	}

	if rsp.StatusCode != 200 {
		return nil, fmt.Errorf("%s", body)
	}

	return body, nil
}