package agent

// runQualityGates runs tierIgnored-specific quality gates in the given work directory.
func (e *Executor) runQualityGates(workDir string, tierIgnored interface{}) []*GateOutput {
	gates := NewQualityGates(workDir)

	// Configure gates based on tierIgnored
	gateConfig := GateConfigForTier(tierIgnored)
	gates.EnableLint(gateConfig.Lint)
	gates.EnableBuild(gateConfig.Build)
	gates.EnableTest(gateConfig.Test)
	gates.EnableTypecheck(gateConfig.TypeCheck)

	// Run the enabled gates
	results, err := gates.RunGates()
	if err != nil {
		// Return a single error result if gate execution itself failed
		return []*GateOutput{{
			Gate:   "gates",
			Result: GateError,
			Output: err.Error(),
		}}
	}

	return results
}

// evaluateGates checks if all enabled gates passed.
func (e *Executor) evaluateGates(gateResults []*GateOutput) bool {
	for _, gate := range gateResults {
		if gate.Result == GateFail || gate.Result == GateError {
			return false
		}
	}
	return true
}

// evaluateGatesWithBaseline checks gates using baseline for regression detection.
// When a baseline is set, pre-existing failures are allowed but new failures block.
func (e *Executor) evaluateGatesWithBaseline(gateResults []*GateOutput, baseline *Baseline) bool {
	// If no baseline, use simple pass/fail
	if baseline == nil {
		return e.evaluateGates(gateResults)
	}

	// Convert gate results to GateResults for baseline comparison
	current := &GateResults{}
	for _, gate := range gateResults {
		if gate.Result != GateFail && gate.Result != GateError {
			continue
		}

		failures := parseGateOutputForFailures(gate.Gate, gate.Output)
		switch gate.Gate {
		case "test":
			current.FailingTests = append(current.FailingTests, failures...)
		case "lint":
			current.LintErrors = append(current.LintErrors, failures...)
		case "typecheck", "build":
			current.TypeErrors = append(current.TypeErrors, failures...)
		}
	}

	// Compare to baseline
	comparison := CompareToBaseline(current, baseline)
	return !comparison.IsRegression
}

// shouldRunRalphLoop determines if the Ralph self-critique loop should run for the given tierIgnored.
// Scout and Quick tiers skip the loop (simple tasks don't need self-critique).
// Builder and Architect tiers run the loop for quality improvement.
// TODO: Implement proper tier checking when tier types are integrated
func (e *Executor) shouldRunRalphLoop(tierIgnored interface{}) bool {
	return true // Default to enabled until tier logic is implemented
}
