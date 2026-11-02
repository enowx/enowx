package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/enowx/enowx/proto"
)

// onboardingFile is under settings/, which syncs: the record follows the user's hosts.
func (s *Server) onboardingFile() string {
	return filepath.Join(s.dataDir, "settings", "onboarding.json")
}

func (s *Server) onboarding() proto.OnboardingState {
	var st proto.OnboardingState
	if data, err := os.ReadFile(s.onboardingFile()); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	return st
}

func (s *Server) setOnboarding(st proto.OnboardingState) (proto.OnboardingState, error) {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return st, err
	}
	if err := os.MkdirAll(filepath.Dir(s.onboardingFile()), 0o700); err != nil {
		return st, err
	}
	return st, writeFileAtomic(s.onboardingFile(), data)
}
