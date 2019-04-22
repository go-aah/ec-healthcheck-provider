// Author: Adrián López (https://github.com/adrianlop)
// Copyright (c) Jeevanandam M. (https://github.com/jeevatkm)
// Source code and usage is governed by a MIT style
// license that can be found in the LICENSE file.

package health

import (
	"fmt"
	"net/http"
	"path"
	"sync"
	"time"

	"aahframe.work"
	"aahframe.work/ainsp"
	"aahframe.work/router"
)

var (
	defaultCollector *Collector
)

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

//‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾
// Collector struct and its methods
//______________________________________________________________________________

// Collector contains the health reporters to check and its responded
// data for the JSON response.
type Collector struct {
	globalHealth  bool
	reporters     map[string]*Config
	reportersData map[string]string
	mu            sync.RWMutex
}

// NewCollector method returns a `Collector` instance. It periodically checks
// all its registered reporters.
func NewCollector(interval time.Duration) *Collector {
	defaultCollector = &Collector{
		reporters:     make(map[string]*Config),
		reportersData: make(map[string]string),
		globalHealth:  true,
	}

	if interval <= 0 {
		// if interval is negative or 0, default to 10s interval checks
		interval = 10
	}
	go func(interval time.Duration) {
		//sleep 5s + do initial runChecks, so we don't wait 10s when app starts
		time.Sleep(5 * time.Second)
		defaultCollector.runChecks()

		// ticker to check reporters periodically using specified interval
		t := time.NewTicker(interval * time.Second)
		for range t.C {
			defaultCollector.runChecks()
		}
	}(interval)

	return defaultCollector
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

	for _, cfg := range c.reporters {
		go func(rc *Config) {
			defer wg.Done()
			//change the dependency health values
			if err := rc.Reporter.Check(); err != nil {
				if !rc.SoftFail {
					c.mu.Lock()
					c.globalHealth = false
					c.mu.Unlock()
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
}

// Register method registers the health collector into aah application with
// two routes `/healthcheck` and `/ping`.
//
// Provides optional base path or route prefix for the above routes.
func (c *Collector) Register(app *aah.Application, basePath ...string) error {
	routePrefix := ""
	if len(basePath) > 0 {
		routePrefix = basePath[0]
	}
	return registerInApp(app, app.Router().RootDomain().Key, routePrefix)
}

// RegisterForDomain method registers the health collector into
// aah application with two routes `/healthcheck` and `/ping` for
// given domain hostname.
//
// Provides optional base path or route prefix for the above routes.
func (c *Collector) RegisterForDomain(app *aah.Application, domainName string, basePath ...string) error {
	routePrefix := ""
	if len(basePath) > 0 {
		routePrefix = basePath[0]
	}
	return registerInApp(app, domainName, routePrefix)
}

func registerInApp(app *aah.Application, domainName, basePath string) error {
	app.AddController((*healthController)(nil), []*ainsp.Method{
		{Name: "Healthcheck"},
		{Name: "Ping"},
	})
	hcRoute := &router.Route{
		Name:   "healthcheck",
		Path:   composeRoutePath(basePath, "healthcheck"),
		Method: http.MethodGet,
		Target: "aahframe.work/ec/health/healthController",
		Action: "Healthcheck",
	}
	if err := app.Router().Lookup(domainName).AddRoute(hcRoute); err != nil {
		return fmt.Errorf("health: cannot add route '%v': %v", hcRoute.Name, err.Error())
	}
	pingRoute := &router.Route{
		Name:   "ping",
		Path:   composeRoutePath(basePath, "ping"),
		Method: http.MethodGet,
		Target: "aahframe.work/ec/health/healthController",
		Action: "Ping",
	}
	if err := app.Router().Lookup(domainName).AddRoute(pingRoute); err != nil {
		return fmt.Errorf("health: cannot add route '%v': %v", hcRoute.Name, err.Error())
	}
	return nil
}

func composeRoutePath(basePath, routePath string) string {
	p := path.Join(basePath, routePath)
	if p[0] == '/' {
		return p
	}
	return "/" + p
}

//‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾
// HealthController struct and its methods
//______________________________________________________________________________

// HealthController provides action methods for health check and ping
// for the aah application.
type healthController struct {
	*aah.Context
}

// TODO: this action should take input parameter *Collector, to support multiple collectors
// Healthcheck action responds with reporter's health status.
func (c *healthController) Healthcheck() {
	defaultCollector.mu.RLock()
	defer defaultCollector.mu.RUnlock()
	if defaultCollector.globalHealth {
		c.Reply().Ok().JSON(defaultCollector.reportersData)
	} else {
		c.Reply().ServiceUnavailable().JSON(defaultCollector.reportersData)
	}
}

// Ping action responds with static text response as `pong!` with status `200 OK`.
func (c *healthController) Ping() {
	c.Reply().Ok().Text("pong!\n")
}
