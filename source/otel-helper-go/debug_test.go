package main

import (
	"os"
	"testing"
)

func TestInitDebug_AcceptedValues(t *testing.T) {
	accepted := []string{"true", "1", "yes", "y", "TRUE", "True", "YES", "Y"}

	for _, val := range accepted {
		t.Run(val, func(t *testing.T) {
			debugMode = false
			os.Setenv("DEBUG_MODE", val)
			defer os.Unsetenv("DEBUG_MODE")

			initDebug()

			if !debugMode {
				t.Errorf("initDebug() with DEBUG_MODE=%q should set debugMode=true", val)
			}
		})
	}
}

func TestInitDebug_RejectedValues(t *testing.T) {
	rejected := []string{"false", "0", "no", "n", "", "maybe", "on"}

	for _, val := range rejected {
		t.Run(val, func(t *testing.T) {
			debugMode = false
			os.Setenv("DEBUG_MODE", val)
			defer os.Unsetenv("DEBUG_MODE")

			initDebug()

			if debugMode {
				t.Errorf("initDebug() with DEBUG_MODE=%q should not set debugMode=true", val)
			}
		})
	}
}

func TestInitDebug_Unset(t *testing.T) {
	debugMode = false
	os.Unsetenv("DEBUG_MODE")

	initDebug()

	if debugMode {
		t.Error("initDebug() with unset DEBUG_MODE should not set debugMode=true")
	}
}
