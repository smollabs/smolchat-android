package app

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Complexity is a coarse estimate of how demanding a request is.
type Complexity int

const (
	Simple Complexity = iota
	Moderate
	Complex
)

func (c Complexity) String() string {
	switch c {
	case Moderate:
		return "moderate"
	case Complex:
		return "complex"
	}
	return "simple"
}

// RouteRequest is what the router knows about a turn.
type RouteRequest struct {
	Query        string
	HistoryTurns int
	NeedsVision  bool
	NeedsDocs    bool
}

// RouteInput carries the environment the router decides in.
type RouteInput struct {
	Installed   []ModelSpec // installed, supported models
	Device      DeviceInfo
	ContextSize int
	LoadedID    string // currently resident model (switching costs a load)
	// Preference shifts the tier: "speed" favours smaller models,
	// "quality" larger ones; anything else is balanced.
	Preference string
}

// Route is the router's decision.
type Route struct {
	Model      ModelSpec
	Complexity Complexity
	Reason     string // short human explanation shown in the UI
}

// ErrNoModel means no installed model can serve the request.
var ErrNoModel = errors.New("no model installed")

var (
	reCode     = regexp.MustCompile("(?i)```|\\b(func|def|class|import|return|const|var|let|SELECT|#include)\\b|[{};]\\s*$|=>")
	reMath     = regexp.MustCompile(`\d+\s*[-+*/^=]\s*\d+|\b(integral|derivative|equation|prove|theorem|probability|matrix)\b`)
	complexKws = []string{
		"explain", "why", "how does", "how do", "analy", "compare", "difference between", "step by step", "step-by-step",
		"write a", "write an", "essay", "story", "poem", "code", "program", "function", "algorithm", "debug", "bug",
		"translate", "summarize", "summarise", "plan", "design", "pros and cons", "reason", "proof", "calculate",
		"implement", "refactor", "optimi", "detailed", "in depth", "outline", "strategy",
	}
	simpleKws = []string{"hi", "hello", "hey", "thanks", "thank you", "ok", "okay", "yes", "no", "good morning", "good night"}
)

// Classify estimates a query's complexity from its length, keywords and
// structure. It is deliberately cheap: it runs on every send.
func Classify(q string, historyTurns int) Complexity {
	t := strings.ToLower(strings.TrimSpace(q))
	words := len(strings.Fields(t))
	score := 0
	switch {
	case words > 60:
		score += 3
	case words > 25:
		score += 2
	case words > 10:
		score++
	}
	hits := 0
	for _, k := range complexKws {
		if strings.Contains(t, k) {
			hits++
		}
	}
	if hits > 0 {
		score += 1 + min(hits, 3) // each extra signal adds weight, capped
	}
	if reCode.MatchString(q) {
		score += 3
	}
	if reMath.MatchString(t) {
		score += 2
	}
	if strings.Count(t, "?") > 1 || strings.Count(q, "\n") > 2 {
		score++
	}
	if historyTurns > 6 {
		score++
	}
	if words <= 4 {
		for _, k := range simpleKws {
			if strings.Trim(t, "!.?, ") == k {
				return Simple
			}
		}
	}
	switch {
	case score >= 4:
		return Complex
	case score >= 2:
		return Moderate
	}
	return Simple
}

// familyRank orders families by quality at equal size.
func familyRank(f string) int {
	switch f {
	case "SmolLM3":
		return 3
	case "SmolLM2":
		return 2
	}
	return 1
}

// SelectModel picks the best installed model for req.
//
// Policy: estimate complexity, then choose the smallest model that matches
// the needed tier among those that fit in memory, preferring the newer family
// and the already-loaded model (switching costs seconds of loading). If
// nothing in the tier is installed, fall back to the closest installed
// model and say so.
func SelectModel(req RouteRequest, in RouteInput) (Route, error) {
	if req.NeedsVision {
		return Route{}, errors.New("image questions need a SmolVLM model, which this version of SmolChat cannot run yet")
	}
	if req.NeedsDocs {
		return Route{}, errors.New("document understanding (SmolDocling) is not available in this version yet")
	}
	nCtx := in.ContextSize
	if nCtx <= 0 {
		nCtx = 2048
	}
	var fits []ModelSpec
	for _, m := range in.Installed {
		if m.Category != CategoryText || !m.Supported {
			continue
		}
		if AssessFit(m, in.Device, nCtx) <= FitTight {
			fits = append(fits, m)
		}
	}
	if len(fits) == 0 {
		if len(in.Installed) == 0 {
			return Route{}, ErrNoModel
		}
		return Route{}, fmt.Errorf("none of your downloaded models fit in this device's memory; try a smaller model or a shorter context")
	}

	cx := Classify(req.Query, req.HistoryTurns)
	tier := cx // the tier we aim for, after the user's preference
	switch {
	case in.Preference == "speed" && tier > Simple:
		tier--
	case in.Preference == "quality" && tier < Complex:
		tier++
	}
	// Desired size tier per complexity (in millions of parameters).
	lo, hi := 0, 400
	switch tier {
	case Moderate:
		lo, hi = 300, 1000
	case Complex:
		lo, hi = 1000, 1<<30
	}

	score := func(m ModelSpec) int {
		s := familyRank(m.Family) * 10
		inTier := m.ParamsM >= lo && m.ParamsM <= hi
		if inTier {
			s += 100
		}
		switch tier {
		case Simple:
			s -= m.ParamsM / 50 // smaller is better
		default:
			s += m.ParamsM / 100 // bigger is better
		}
		if AssessFit(m, in.Device, nCtx) == FitTight {
			s -= 40
		}
		if in.Device.Thermal >= ThermalSevere {
			s -= m.ParamsM / 40 // throttling: favour small models
		}
		if m.ID == in.LoadedID {
			s += 25 // avoid a reload when it is a close call
		}
		return s
	}
	best := fits[0]
	for _, m := range fits[1:] {
		if score(m) > score(best) {
			best = m
		}
	}

	reason := ""
	switch cx {
	case Simple:
		reason = "quick question"
	case Moderate:
		reason = "everyday question"
	default:
		reason = "complex request"
	}
	if best.ParamsM < lo {
		reason += " · a larger model would do better"
	}
	return Route{Model: best, Complexity: cx, Reason: reason}, nil
}

// Recommend returns the best supported text model for the device, used to
// guide first-time users. It prefers the official SmolLM2 360M where it fits.
func Recommend(d DeviceInfo, nCtx int) ModelSpec {
	var best ModelSpec
	for _, id := range []string{"smollm2-360m-instruct-q8", "smollm2-135m-instruct-q8"} {
		m, _ := FindSpec(id)
		if AssessFit(m, d, nCtx) <= FitOK {
			return m
		}
		best = m
	}
	return best
}
