package pid

import (
	"math"
	"sync"
	"time"
)

type Controller struct {
	Kp                        float64
	Ki                        float64
	Kd                        float64
	Setpoint                  float64
	OutputMin                 float64
	OutputMax                 float64
	Integral                  float64
	PrevError                 float64
	PrevTime                  time.Time
	FirstRun                  bool
	IntegralSeparationPercent float64
	mu                        sync.Mutex
}

func NewController(kp, ki, kd, setpoint, outputMin, outputMax float64) *Controller {
	return &Controller{
		Kp:                        kp,
		Ki:                        ki,
		Kd:                        kd,
		Setpoint:                  setpoint,
		OutputMin:                 outputMin,
		OutputMax:                 outputMax,
		FirstRun:                  true,
		IntegralSeparationPercent: 20.0,
	}
}

func (c *Controller) SetIntegralSeparationThreshold(percent float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if percent > 0 && percent <= 100 {
		c.IntegralSeparationPercent = percent
	}
}

func (c *Controller) ResetIntegral() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Integral = 0
}

func (c *Controller) Compute(actual float64, now time.Time) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	error := c.Setpoint - actual

	if c.FirstRun {
		c.PrevError = error
		c.PrevTime = now
		c.FirstRun = false
		return c.clamp(c.Kp * error)
	}

	dt := now.Sub(c.PrevTime).Seconds()
	if dt <= 0 {
		dt = 1.0
	}

	deviationPercent := math.Abs(error) / c.Setpoint * 100
	separationThreshold := c.IntegralSeparationPercent

	if deviationPercent <= separationThreshold {
		proposedIntegral := c.Integral + error*dt

		termP := c.Kp * error
		termI := c.Ki * proposedIntegral
		termD := c.Kd * ((error - c.PrevError) / dt)
		proposedOutput := termP + termI + termD
		clampedProposed := c.clamp(proposedOutput)

		if proposedOutput == clampedProposed {
			c.Integral = proposedIntegral
		} else {
			sameDirection := (error > 0 && proposedOutput > clampedProposed) || (error < 0 && proposedOutput < clampedProposed)
			if !sameDirection {
				c.Integral = proposedIntegral
			}
		}
	}

	derivative := (error - c.PrevError) / dt
	output := c.Kp*error + c.Ki*c.Integral + c.Kd*derivative
	clampedOutput := c.clamp(output)

	c.PrevError = error
	c.PrevTime = now

	return clampedOutput
}

func (c *Controller) ComputeWithFeedforward(actual, feedforward float64, now time.Time) float64 {
	pidOutput := c.Compute(actual, now)
	return c.clamp(pidOutput + feedforward)
}

func (c *Controller) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Integral = 0
	c.PrevError = 0
	c.FirstRun = true
}

func (c *Controller) SetSetpoint(sp float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Setpoint = sp
}

func (c *Controller) SetTunings(kp, ki, kd float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Kp = kp
	c.Ki = ki
	c.Kd = kd
}

func (c *Controller) SetOutputLimits(min, max float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.OutputMin = min
	c.OutputMax = max
}

func (c *Controller) clamp(output float64) float64 {
	if output > c.OutputMax {
		return c.OutputMax
	}
	if output < c.OutputMin {
		return c.OutputMin
	}
	return output
}

type AntiWindupController struct {
	*Controller
	Kb float64
}

func NewAntiWindupController(kp, ki, kd, kb, setpoint, outputMin, outputMax float64) *AntiWindupController {
	return &AntiWindupController{
		Controller: NewController(kp, ki, kd, setpoint, outputMin, outputMax),
		Kb:         kb,
	}
}

func (c *AntiWindupController) Compute(actual float64, now time.Time) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	error := c.Setpoint - actual

	if c.FirstRun {
		c.PrevError = error
		c.PrevTime = now
		c.FirstRun = false
		return c.clamp(c.Kp * error)
	}

	dt := now.Sub(c.PrevTime).Seconds()
	if dt <= 0 {
		dt = 1.0
	}

	preSat := c.Kp*error + c.Ki*c.Integral + c.Kd*((error-c.PrevError)/dt)
	output := c.clamp(preSat)

	if c.Ki > 0 {
		c.Integral += (error + c.Kb*(output-preSat)) * dt
	}

	c.PrevError = error
	c.PrevTime = now

	return output
}

type ParallelPID struct {
	KP        float64
	KI        float64
	KD        float64
	Setpoint  float64
	OutMin    float64
	OutMax    float64
	Deadband  float64

	integral   float64
	prevError  float64
	prevTime   time.Time
	firstRun   bool
	mu         sync.Mutex
}

func NewParallelPID(kp, ki, kd, setpoint, min, max float64) *ParallelPID {
	return &ParallelPID{
		KP:       kp,
		KI:       ki,
		KD:       kd,
		Setpoint: setpoint,
		OutMin:   min,
		OutMax:   max,
		Deadband: 0.0,
		firstRun: true,
	}
}

func (p *ParallelPID) Update(processValue float64, now time.Time) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	err := p.Setpoint - processValue

	if math.Abs(err) < p.Deadband {
		err = 0
	}

	if p.firstRun {
		p.prevError = err
		p.prevTime = now
		p.firstRun = false
		return p.clamp(p.KP * err)
	}

	dt := now.Sub(p.prevTime).Seconds()
	if dt <= 0 {
		dt = 1.0
	}

	termP := p.KP * err
	termI := p.integral + p.KI*err*dt
	termD := p.KD * (err - p.prevError) / dt

	preOutput := termP + termI + termD
	output := p.clamp(preOutput)

	if output != preOutput && p.KI != 0 {
		termI = output - termP - termD
	}

	p.integral = termI
	p.prevError = err
	p.prevTime = now

	return output
}

func (p *ParallelPID) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.integral = 0
	p.prevError = 0
	p.firstRun = true
}

func (p *ParallelPID) SetSetpoint(sp float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Setpoint = sp
}

func (p *ParallelPID) clamp(v float64) float64 {
	if v > p.OutMax {
		return p.OutMax
	}
	if v < p.OutMin {
		return p.OutMin
	}
	return v
}
