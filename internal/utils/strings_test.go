package utils_test

import (
	"testing"

	"github.com/temirov/llm-proxy/internal/constants"
	"github.com/temirov/llm-proxy/internal/utils"
)

const (
	emptyStringValue      = constants.EmptyString
	whitespaceStringValue = " \t\n"
	wordStringValue       = "hello"
	spacedWordStringValue = "  hello  "
	valueFoobar           = "foobar"
	prefixFoo             = "foo"
	prefixFooUppercase    = "FOO"
	prefixBar             = "bar"
	prefixBaz             = "baz"
)

type isBlankTestDefinition struct {
	testName      string
	inputValue    string
	expectedValue bool
}

// TestIsBlank_IdentifiesBlankStrings verifies that IsBlank correctly identifies blank strings.
func TestIsBlank_IdentifiesBlankStrings(testingInstance *testing.T) {
	testCases := []isBlankTestDefinition{
		{testName: "empty string", inputValue: emptyStringValue, expectedValue: true},
		{testName: "whitespace string", inputValue: whitespaceStringValue, expectedValue: true},
		{testName: "word string", inputValue: wordStringValue, expectedValue: false},
		{testName: "spaced word string", inputValue: spacedWordStringValue, expectedValue: false},
	}
	for _, currentTestCase := range testCases {
		testingInstance.Run(currentTestCase.testName, func(nestedTestingInstance *testing.T) {
			actualBlank := utils.IsBlank(currentTestCase.inputValue)
			if actualBlank != currentTestCase.expectedValue {
				nestedTestingInstance.Fatalf("blank=%v expected=%v", actualBlank, currentTestCase.expectedValue)
			}
		})
	}
}

type hasAnyPrefixTestDefinition struct {
	testName      string
	value         string
	prefixes      []string
	expectedValue bool
}

// TestHasAnyPrefix_DetectsPrefixes verifies that HasAnyPrefix detects matching prefixes in a case-insensitive manner.
func TestHasAnyPrefix_DetectsPrefixes(testingInstance *testing.T) {
	testCases := []hasAnyPrefixTestDefinition{
		{testName: "direct match", value: valueFoobar, prefixes: []string{prefixFoo}, expectedValue: true},
		{testName: "case insensitive match", value: valueFoobar, prefixes: []string{prefixFooUppercase}, expectedValue: true},
		{testName: "multiple prefixes no match", value: valueFoobar, prefixes: []string{prefixBar, prefixBaz}, expectedValue: false},
	}
	for _, currentTestCase := range testCases {
		testingInstance.Run(currentTestCase.testName, func(nestedTestingInstance *testing.T) {
			actualHasPrefix := utils.HasAnyPrefix(currentTestCase.value, currentTestCase.prefixes...)
			if actualHasPrefix != currentTestCase.expectedValue {
				nestedTestingInstance.Fatalf("hasPrefix=%v expected=%v", actualHasPrefix, currentTestCase.expectedValue)
			}
		})
	}
}
