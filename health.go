package health // import "aahframe.work/ec/health"

import (
	aah "aahframe.work"
	"fmt"
	"sync"
	"time"
)

// Collector contains the dependencies to check
type Collector struct {
	reporters       map[string]*Config
	globalHealth    bool
	globalHealthMsg map[string]string
	mu              sync.RWMutex
}

// Reporter is the interface for a dependency that can be health-checked
type Reporter interface {
	// Check will return nil if dependency is reachable/healthy
	// You should implement this func with a sensible timeout (< 3 or 5 sec)
	Check() error
}

// Config struct contains a Reporter configuration
type Config struct {
	Name     string
	Reporter Reporter
	// SoftDep - if true it will allow errors so won't report unhealthy
	SoftFail bool
	// TODO: interval unused for now - got a global interval for all checks
	// interval time.Duration
}

// NewCollector returns a Collector and periodically checks all its registered reporters
func NewCollector(interval time.Duration) *Collector {
	if interval <= 0 {
		// if interval is negative or 0, default to 10s interval checks
		interval = 10
	}
	collector := &Collector{
		reporters:    make(map[string]*Config),
		globalHealth: true,
	}
	go func() {
		t := time.NewTicker(interval * time.Second)
		for {
			<-t.C
			collector.runChecks()
		}
	}()

	return collector
}

// AddReporter adds a dependency to check it periodically
func (c *Collector) AddReporter(config *Config) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.reporters[config.Name]
	if exists {
		return fmt.Errorf("Reporter name '%s' already exists", config.Name)
	}
	c.reporters[config.Name] = config
	return nil
}

// RunChecks performs a check in all the dependencies and update the global status
func (c *Collector) runChecks() {
	//create syncgroup and check all dependencies
	var wg sync.WaitGroup
	wg.Add(len(c.reporters))

	depsHealth := make(map[string]string)
	unhealthy := 0
	c.mu.RLock()
	for name := range c.reporters {
		go func(name string) {
			defer wg.Done()
			//change the dependency health values
			err := c.reporters[name].Reporter.Check()
			if err != nil {
				// increment unhealthy counter if it's a hard dependency
				if !c.reporters[name].SoftFail {
					unhealthy++
				}
				depsHealth[name] = "KO: " + err.Error()
			} else {
				depsHealth[name] = "OK: Healthy"
			}

		}(name)
	}
	c.mu.RUnlock()
	// wait for all the deps to finish the checks
	wg.Wait()

	// calculate the globalHealth (if 1+ services are unhealthy, globalHealth is unhealthy too)
	globalHealth := true
	if unhealthy != 0 {
		globalHealth = false
	}
	// refresh the calculated globalHealth
	c.mu.Lock()
	c.globalHealth = globalHealth
	c.globalHealthMsg = depsHealth
	c.mu.Unlock()
}

// Register the collector in Aah application
func (c *Collector) Register(app *aah.Application) {

}

// RegisterForDomain registers the collector in Aah specified domain
func (c *Collector) RegisterForDomain(app *aah.Application, domainName string) {

}

// Controller only has the aah HTTP Context
type healthController struct {
	*aah.Context
}

// HealthcheckHandler is an Aah handler to report the healthcheck
func (c *healthController) HealthcheckHandler(hc *Collector) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	if hc.globalHealth {
		c.Reply().Ok().JSON(hc.globalHealthMsg)
	} else {
		c.Reply().ServiceUnavailable().JSON(hc.globalHealthMsg)
	}
}

// Ping is an Aah handler for always returning 200 OK
func (c *healthController) Ping() {
	c.Reply().Ok().Text("pong!")
}
