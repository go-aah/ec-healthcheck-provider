// Author: Adrián López (https://github.com/adrianlop)
// Copyright (c) Jeevanandam M. (https://github.com/jeevatkm)
// Source code and usage is governed by a MIT style
// license that can be found in the LICENSE file.

package health

import (
	"fmt"
	"sync"
	"time"

	"aahframe.work"
)

// Collector contains the health reporters to check and its responded
// data for the JSON response.
type Collector struct {
	globalHealth  bool
	reporters     map[string]*Config
	reportersData map[string]string
	mu            sync.RWMutex
}

// Reporter interface for a dependency that can be health-checked.
type Reporter interface {
	// Check will return nil if dependency is reachable/healthy
	// You should implement this func with a sensible timeout (< 3 or 5 sec)
	Check() error
}

// Config struct contains a Reporter configuration
type Config struct {
	Name     string
	Reporter Reporter
	SoftFail bool // if true it will allow errors so won't report unhealthy
}

// NewCollector method returns a `Collector` instance. It periodically checks
// all its registered reporters.
func NewCollector(interval time.Duration) *Collector {
	if interval <= 0 {
		// if interval is negative or 0, default to 10s interval checks
		interval = 10
	}
	collector := &Collector{
		reporters:     make(map[string]*Config),
		reportersData: make(map[string]string),
		globalHealth:  true,
	}
	go func() {
		t := time.NewTicker(interval * time.Second)
		for range t.C {
			collector.runChecks()
		}
	}()

	return collector
}

// AddReporter method adds a dependency to health check reporter
// that will be called per interval to get health report.
func (c *Collector) AddReporter(config *Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.reporters[config.Name]; exists {
		return fmt.Errorf("health: reporter name '%s' already exists", config.Name)
	}
	c.reporters[config.Name] = config
	return nil
}

// RunChecks method performs a check in all the dependencies and update the global status
func (c *Collector) runChecks() {
	//create syncgroup and check all dependencies
	var wg sync.WaitGroup
	c.mu.RLock()
	wg.Add(len(c.reporters))
	unhealthy := 0

	for _, cfg := range c.reporters {
		go func(rc *Config) {
			defer wg.Done()
			//change the dependency health values
			if err := rc.Reporter.Check(); err != nil {
				if !rc.SoftFail {
					// increment unhealthy counter if it's a hard dependency
					unhealthy++
				}
				c.mu.Lock()
				c.reportersData[rc.Name] = "KO: " + err.Error()
				c.mu.Unlock()
			} else {
				c.mu.Lock()
				c.reportersData[rc.Name] = "OK: Healthy"
				c.mu.Unlock()
			}
		}(cfg)
	}
	c.mu.RUnlock()

	// wait for all the deps to finish the checks
	wg.Wait()

	// refresh the calculated globalHealth
	c.mu.Lock()
	// calculate the globalHealth (if 1+ services are unhealthy, globalHealth is unhealthy too)
	c.globalHealth = unhealthy == 0
	c.mu.Unlock()
}

// Register the collector in aah application
func (c *Collector) Register(app *aah.Application) error {
	return nil
}

// RegisterForDomain registers the collector in aah specified domain
func (c *Collector) RegisterForDomain(app *aah.Application, domainName string) error {
	return nil
}

// HealthController provides an action methods for health check and ping
// for the aah application.
type healthController struct {
	*aah.Context
}

// TODO this action may take input paramter, stil its not finalized....

// Healthcheck action responds with reporters health state to the caller.
func (c *healthController) Healthcheck(hc *Collector) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	if hc.globalHealth {
		c.Reply().Ok().JSON(hc.reportersData)
	} else {
		c.Reply().ServiceUnavailable().JSON(hc.reportersData)
	}
}

// Ping action responds with static text response as `pong!` with
// status `200 OK` to the caller.
func (c *healthController) Ping() {
	c.Reply().Ok().Text("pong!")
}
