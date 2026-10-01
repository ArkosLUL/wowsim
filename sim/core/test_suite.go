package core

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"google.golang.org/protobuf/encoding/prototext"
)

// Precise enough to detect very small changes to test results, but truncated
// enough that we don't have flaky tests due to different OS/Go versions with
// different float rounding behavior.
const storagePrecision = 5

type IndividualTestSuite struct {
	Name string

	// Names of all the tests, in the order they are tested.
	testNames []string

	testResults *proto.TestSuiteResult
}

func NewIndividualTestSuite(suiteName string) *IndividualTestSuite {
	return &IndividualTestSuite{
		Name:        suiteName,
		testResults: newTestSuiteResult(),
	}
}

func (testSuite *IndividualTestSuite) TestCharacterStats(testName string, csr *proto.ComputeStatsRequest) {
	testSuite.testNames = append(testSuite.testNames, testName)

	result := ComputeStats(csr)
	finalStats := stats.FromFloatArray(result.RaidStats.Parties[0].Players[0].FinalStats.Stats)

	testSuite.testResults.CharacterStatsResults[testName] = &proto.CharacterStatsTestResult{
		FinalStats: toFixedStats(finalStats[:], storagePrecision),
	}
}

func (testSuite *IndividualTestSuite) TestStatWeights(testName string, swr *proto.StatWeightsRequest) {
	testSuite.testNames = append(testSuite.testNames, testName)

	result := StatWeights(swr)
	weights := stats.FromFloatArray(result.Dps.Weights.Stats)

	testSuite.testResults.StatWeightsResults[testName] = &proto.StatWeightsTestResult{
		Weights: toFixedStats(weights[:], storagePrecision),
	}
}

func (testSuite *IndividualTestSuite) TestDPS(testName string, rsr *proto.RaidSimRequest) {
	testSuite.testNames = append(testSuite.testNames, testName)

	result := RunRaidSim(rsr)
	if result.Logs != "" {
		fmt.Printf("LOGS: %s\n", result.Logs)
	}
	if result.ErrorResult != "" {
		panic("simulation failed to run: " + result.ErrorResult)
	}
	testSuite.testResults.DpsResults[testName] = &proto.DpsTestResult{
		Dps:  toFixed(result.RaidMetrics.Dps.Avg, storagePrecision),
		Tps:  toFixed(result.RaidMetrics.Parties[0].Players[0].Threat.Avg, storagePrecision),
		Dtps: toFixed(result.RaidMetrics.Parties[0].Players[0].Dtps.Avg, storagePrecision),
		Hps:  toFixed(result.RaidMetrics.Parties[0].Players[0].Hps.Avg, storagePrecision),
	}
	if strings.Contains(testName, "Average") {
		testSuite.testResults.AbilityResults[testName] = abilityResults(result, rsr.SimOptions.Iterations)
	}
}

type abilitySums struct {
	casts, hits, crits, amount, uptime float64
}

// abilityResults reads the player's and its pets' actions and auras, and the targets' auras, off
// one run. Same-named pets and repeated ids add up, so a pet-count change shows as one row moving.
func abilityResults(result *proto.RaidSimResult, iterations int32) *proto.AbilitiesTestResult {
	sums := make(map[string]*abilitySums)
	sumsFor := func(unit, kind string, id *proto.ActionID) *abilitySums {
		key := unit + "|" + kind + "|" + ProtoToActionID(id).String()
		s, ok := sums[key]
		if !ok {
			s = &abilitySums{}
			sums[key] = s
		}
		return s
	}
	addUnit := func(unit string, metrics *proto.UnitMetrics, withActions bool) {
		if withActions {
			for _, action := range metrics.Actions {
				s := sumsFor(unit, "action", action.Id)
				for _, target := range action.Targets {
					s.casts += float64(target.Casts)
					// landed, as the UI counts it: glances and crushes land too
					s.hits += float64(target.Hits + target.Crits + target.Blocks + target.Glances + target.Crushes)
					s.crits += float64(target.Crits)
					s.amount += target.Damage + target.Healing + target.Shielding
				}
			}
		}
		for _, aura := range metrics.Auras {
			sumsFor(unit, "aura", aura.Id).uptime += aura.UptimeSecondsAvg
		}
	}

	player := result.RaidMetrics.Parties[0].Players[0]
	addUnit("player", player, true)
	for _, pet := range player.Pets {
		addUnit("pet:"+pet.Name, pet, true)
	}
	for i, target := range result.EncounterMetrics.Targets {
		addUnit(fmt.Sprintf("target%d", i), target, false)
	}

	iters := float64(iterations)
	entries := make(map[string]*proto.AbilityTestResult, len(sums))
	for key, s := range sums {
		isAction := strings.Contains(key, "|action|")
		if isAction && s.casts == 0 && s.hits == 0 && s.amount == 0 || !isAction && s.uptime == 0 {
			continue
		}
		// amount on its own, since a dot whose ticks don't count as hits has no amount per hit
		entry := &proto.AbilityTestResult{
			Casts:   toFixed(s.casts/iters, storagePrecision),
			Hits:    toFixed(s.hits/iters, storagePrecision),
			UptimeS: toFixed(s.uptime, storagePrecision),
			Amount:  toFixed(s.amount/iters, storagePrecision),
		}
		if s.hits > 0 {
			entry.CritPct = toFixed(s.crits/s.hits*100, storagePrecision)
			entry.AmountPerHit = toFixed(s.amount/s.hits, storagePrecision)
		}
		entries[key] = entry
	}
	return &proto.AbilitiesTestResult{Entries: entries}
}

func within(a, b float64) bool {
	return math.Abs(a-b) <= tolerance
}

// compareAbilities fails t for every key either side has alone, and every field that differs.
func compareAbilities(t *testing.T, expected, actual *proto.AbilitiesTestResult) {
	for key, got := range actual.Entries {
		want, ok := expected.Entries[key]
		if !ok {
			t.Logf("Unexpected ability %s: %v", key, got)
			t.Fail()
			continue
		}
		if !within(got.Casts, want.Casts) || !within(got.Hits, want.Hits) || !within(got.CritPct, want.CritPct) ||
			!within(got.AmountPerHit, want.AmountPerHit) || !within(got.UptimeS, want.UptimeS) ||
			!within(got.Amount, want.Amount) {
			t.Logf("Ability %s expected %v but was %v", key, want, got)
			t.Fail()
		}
	}
	for key, want := range expected.Entries {
		if _, ok := actual.Entries[key]; !ok {
			t.Logf("Missing ability %s: %v", key, want)
			t.Fail()
		}
	}
}

func (testSuite *IndividualTestSuite) Done(t *testing.T) {
	testSuite.writeToFile()
}

const tolerance = 0.00001

func (testSuite *IndividualTestSuite) writeToFile() {
	str := prototext.Format(testSuite.testResults)
	// For some reason the formatter sometimes outputs 2 spaces instead of one.
	// Replace so we get consistent output.
	str = strings.ReplaceAll(str, "  ", " ")
	data := []byte(str)

	err := os.WriteFile(testSuite.Name+".results.tmp", data, 0644)
	if err != nil {
		panic(err)
	}
}

func (testSuite *IndividualTestSuite) readExpectedResults() (*proto.TestSuiteResult, error) {
	data, err := os.ReadFile(testSuite.Name + ".results")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newTestSuiteResult(), nil
		}
		return nil, err
	}

	results := &proto.TestSuiteResult{}
	if err = prototext.Unmarshal(data, results); err != nil {
		return nil, err
	}
	return results, err
}

func newTestSuiteResult() *proto.TestSuiteResult {
	return &proto.TestSuiteResult{
		CharacterStatsResults: make(map[string]*proto.CharacterStatsTestResult),
		StatWeightsResults:    make(map[string]*proto.StatWeightsTestResult),
		DpsResults:            make(map[string]*proto.DpsTestResult),
		AbilityResults:        make(map[string]*proto.AbilitiesTestResult),
	}
}

type TestGenerator interface {
	// The total number of tests that this generator can generate.
	NumTests() int

	// The name and API request for the test with the given index.
	GetTest(testIdx int) (string, *proto.ComputeStatsRequest, *proto.StatWeightsRequest, *proto.RaidSimRequest)
}

func RunTestSuite(t *testing.T, suiteName string, generator TestGenerator) {
	testSuite := NewIndividualTestSuite(suiteName)
	var currentTestName string

	defer func() {
		if p := recover(); p != nil {
			panic(fmt.Sprintf("Panic during test %s: %v", currentTestName, p))
		}
	}()

	expectedResults, err := testSuite.readExpectedResults()
	if err != nil {
		t.Logf("\n\n----- FAILURE LOADING RESULTS FILE TESTS WILL FAIL-----\n%s\n-----\n\n", err)
		t.Fail()
	}

	numTests := generator.NumTests()
	for i := 0; i < numTests; i++ {
		testName, csr, swr, rsr := generator.GetTest(i)
		if strings.Contains(testName, "Average") && testing.Short() {
			continue
		}
		currentTestName = testName

		t.Run(currentTestName, func(t *testing.T) {
			fullTestName := suiteName + "-" + testName
			if csr != nil {
				testSuite.TestCharacterStats(fullTestName, csr)
				if actualCharacterStats, ok := testSuite.testResults.CharacterStatsResults[fullTestName]; ok {
					actualStats := stats.FromFloatArray(actualCharacterStats.FinalStats)
					if expectedCharacterStats, ok := expectedResults.CharacterStatsResults[fullTestName]; ok {
						expectedStats := stats.FromFloatArray(expectedCharacterStats.FinalStats)
						if !actualStats.EqualsWithTolerance(expectedStats, tolerance) {
							t.Logf("Stats expected %v but was %v", expectedStats, actualStats)
							t.Fail()
						}
					} else {
						t.Logf("Unexpected test %s with stats: %v", fullTestName, actualStats)
						t.Fail()
					}
				} else if !ok {
					t.Logf("Missing Result for test %s", fullTestName)
					t.Fail()
				}
			} else if swr != nil {
				testSuite.TestStatWeights(fullTestName, swr)
				if actualStatWeights, ok := testSuite.testResults.StatWeightsResults[fullTestName]; ok {
					actualWeights := stats.FromFloatArray(actualStatWeights.Weights)
					if expectedStatWeights, ok := expectedResults.StatWeightsResults[fullTestName]; ok {
						expectedWeights := stats.FromFloatArray(expectedStatWeights.Weights)
						if !actualWeights.EqualsWithTolerance(expectedWeights, tolerance) {
							t.Logf("Weights expected %v but was %v", expectedWeights, actualWeights)
							t.Fail()
						}
					} else {
						t.Logf("Unexpected test %s with stat weights: %v", fullTestName, actualWeights)
						t.Fail()
					}
				} else if !ok {
					t.Logf("Missing Result for test %s", fullTestName)
					t.Fail()
				}
			} else if rsr != nil {
				testSuite.TestDPS(fullTestName, rsr)
				if actualDpsResult, ok := testSuite.testResults.DpsResults[fullTestName]; ok {
					if expectedDpsResult, ok := expectedResults.DpsResults[fullTestName]; ok {
						// Check whichever of DPS/HPS is larger first, so we get better test diff printouts.
						if actualDpsResult.Dps < actualDpsResult.Hps {
							if actualDpsResult.Hps < expectedDpsResult.Hps-tolerance || actualDpsResult.Hps > expectedDpsResult.Hps+tolerance {
								t.Logf("HPS expected %0.03f but was %0.03f!.", expectedDpsResult.Hps, actualDpsResult.Hps)
								t.Fail()
							}
						}
						if actualDpsResult.Dps < expectedDpsResult.Dps-tolerance || actualDpsResult.Dps > expectedDpsResult.Dps+tolerance {
							t.Logf("DPS expected %0.03f but was %0.03f!.", expectedDpsResult.Dps, actualDpsResult.Dps)
							t.Fail()
						}
						if actualDpsResult.Dps >= actualDpsResult.Hps {
							if actualDpsResult.Hps < expectedDpsResult.Hps-tolerance || actualDpsResult.Hps > expectedDpsResult.Hps+tolerance {
								t.Logf("HPS expected %0.03f but was %0.03f!.", expectedDpsResult.Hps, actualDpsResult.Hps)
								t.Fail()
							}
						}

						if actualDpsResult.Tps < expectedDpsResult.Tps-tolerance || actualDpsResult.Tps > expectedDpsResult.Tps+tolerance {
							t.Logf("TPS expected %0.03f but was %0.03f!.", expectedDpsResult.Tps, actualDpsResult.Tps)
							t.Fail()
						}
						if actualDpsResult.Dtps < expectedDpsResult.Dtps-tolerance || actualDpsResult.Dtps > expectedDpsResult.Dtps+tolerance {
							t.Logf("DTPS expected %0.03f but was %0.03f!.", expectedDpsResult.Dtps, actualDpsResult.Dtps)
							t.Fail()
						}
						if actualAbilities, ok := testSuite.testResults.AbilityResults[fullTestName]; ok {
							if expectedAbilities, ok := expectedResults.AbilityResults[fullTestName]; ok {
								compareAbilities(t, expectedAbilities, actualAbilities)
							} else {
								t.Logf("Unexpected abilities for test %s", fullTestName)
								t.Fail()
							}
						} else if expectedAbilities, ok := expectedResults.AbilityResults[fullTestName]; ok {
							t.Logf("Missing abilities for test %s: %d entries expected", fullTestName, len(expectedAbilities.Entries))
							t.Fail()
						}
					} else {
						t.Logf("Unexpected test %s with %0.03f DPS!", fullTestName, actualDpsResult.Dps)
						t.Fail()
					}
				} else if !ok {
					t.Logf("Missing Result for test %s", fullTestName)
					t.Fail()
				}
			} else {
				panic("No test request provided")
			}
		})
	}

	testSuite.Done(t)

	if t.Failed() {
		t.Log("One or more tests failed! If the changes are intentional, update the expected results with 'make test && make update-tests'. Otherwise go fix your bugs!")
	}
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func toFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}

func toFixedStats(s []float64, precision int) []float64 {
	for i, val := range s {
		s[i] = toFixed(val, precision)
	}
	return s
}
