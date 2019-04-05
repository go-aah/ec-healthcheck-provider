// Author: Adrián López (https://github.com/adrianlop)
// Copyright (c) Jeevanandam M. (https://github.com/jeevatkm)
// Source code and usage is governed by a MIT style
// license that can be found in the LICENSE file.

package health

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type tcp struct {
	address string
}

func (s *tcp) Check() error {
	conn, err := net.DialTimeout("tcp", s.address, 3*time.Second)
	if err == nil {
		conn.Close()
	}
	return err
}

func TestHealthSimple(t *testing.T) {
	collector := NewCollector(-1)
	googleDNS := &tcp{
		address: "google.com:443",
	}
	rep1 := &Config{
		Name:     "GoogleDNS",
		Reporter: googleDNS,
		SoftFail: true,
	}

	// Assert that rep1 is not already added with same name
	err := collector.AddReporter(rep1)
	assert.Nil(t, err)

	time.Sleep(20 * time.Second) // let's wait for time ticker to run

	collector.mu.RLock()
	assert.True(t, collector.globalHealth)

	// assert global JSON msg status
	healthMsg, _ := json.Marshal(collector.reportersData)
	collector.mu.RUnlock()
	assert.JSONEq(t, `{"GoogleDNS":"OK: Healthy"}`, string(healthMsg))
}

func TestHealthForceCheck(t *testing.T) {
	// Do not use NewCollector here, since datarace would occur
	// between maunal vs ticker run
	collector := &Collector{
		reporters:     make(map[string]*Config),
		reportersData: make(map[string]string),
		globalHealth:  true,
	}

	googleDNS := &tcp{
		address: "google.com:443",
	}
	rep1 := &Config{
		Name:     "GoogleDNS",
		Reporter: googleDNS,
		SoftFail: true,
	}

	// Assert that rep1 is not already added with same name
	err := collector.AddReporter(rep1)
	assert.Nil(t, err)
	// immediately run checks
	collector.runChecks()

	// assert globalHealth
	assert.True(t, collector.globalHealth)

	// assert global JSON msg status
	healthMsg, _ := json.Marshal(collector.reportersData)
	assert.JSONEq(t, `{"GoogleDNS":"OK: Healthy"}`, string(healthMsg))

	// Assert that adding rep2 with same name as rep1 will throw err
	rep2 := &Config{
		Name:     "GoogleDNS",
		Reporter: googleDNS,
		SoftFail: true,
	}
	err = collector.AddReporter(rep2)
	assert.NotNil(t, err)

	// Assert rep3 check fails
	googleFakePort := &tcp{
		address: "google.com:12345",
	}
	rep3 := &Config{
		Name:     "GoogleFakePort",
		Reporter: googleFakePort,
		SoftFail: false,
	}
	err = collector.AddReporter(rep3)
	assert.Nil(t, err)
	collector.runChecks()
	assert.False(t, collector.globalHealth)
	healthMsg, _ = json.Marshal(collector.reportersData)
	assert.Contains(t, string(healthMsg), `"GoogleFakePort":"KO: dial tcp`)

	// TODO: some more testcases
	// testcases := []struct {
	// 	caseName string
	// 	expected interface{}
	// }{
	// 	{
	// 		caseName: "testX",
	// 		expected: nil,
	// 	},
	// }
	// for _, tc := range testcases {
	// assert stuff
	// }
}
