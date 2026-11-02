package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/enowx/enowx/core/agent"
	"github.com/enowx/enowx/core/android"
	"github.com/enowx/enowx/proto"
)

// The host's Android devices (docs/plans/mobile.md): what it has, the
// emulators the daemon started, devices' logs, and a session's app run on
// one in a tab.

// androidState is what the daemon keeps of Android. Its fields belong to
// mu; logs is safe for use by several goroutines itself.
type androidState struct {
	mu        sync.Mutex
	emulators map[string]*exec.Cmd         // started here, by AVD
	mirrors   map[string]*android.Mirror   // screens shown, by serial
	controls  map[string][]android.Control // the controls last read, by serial
	logs      android.Logs
}

func (s *Server) androidStatus() proto.AndroidStatus {
	st := android.Status()
	st.Display = needsDisplay()
	// Emulators starting show before adb sees them.
	s.android.mu.Lock()
	defer s.android.mu.Unlock()
	for avd, cmd := range s.android.emulators {
		if cmd.ProcessState != nil {
			delete(s.android.emulators, avd)
			continue
		}
		online := false
		for _, d := range st.Devices {
			if d.Kind == "emulator" && d.Name == avd {
				online = true
			}
		}
		if !online {
			st.Devices = append(st.Devices, proto.AndroidDevice{Kind: "emulator", Name: avd, State: "starting"})
		}
	}
	return st
}

// androidSetup sets Android up in a tab of session p.Session, where the
// SDK's licenses are read and accepted.
func (s *Server) androidSetup(p proto.AndroidSetupParams) (proto.SessionInfo, error) {
	if p.Session == "" {
		return proto.SessionInfo{}, errors.New("android: set up from a session")
	}
	tab, err := s.createSession(proto.SessionCreateParams{
		Parent: p.Session, Name: "Android setup", Agent: agent.Custom, Command: thenShell(setupArgs(programPath(), p.TV)),
	})
	if err != nil {
		return proto.SessionInfo{}, err
	}
	return tab.Info(), nil
}

func (s *Server) androidSDK() (android.SDK, error) {
	sdk, ok := android.FindSDK()
	if !ok {
		return sdk, errors.New("android: no Android SDK on this host: Set up Android first")
	}
	return sdk, nil
}

// androidStart starts a virtual device, with its own window only where
// this host has a screen.
func (s *Server) androidStart(p proto.AndroidDeviceParams) error {
	sdk, err := s.androidSDK()
	if err != nil {
		return err
	}
	s.android.mu.Lock()
	if cmd, ok := s.android.emulators[p.AVD]; ok && cmd.ProcessState == nil {
		s.android.mu.Unlock()
		return nil
	}
	s.android.mu.Unlock()
	sdk.StartServer()
	cmd, err := sdk.Start(p.AVD, p.Window && !needsDisplay())
	if err != nil {
		return err
	}
	s.android.mu.Lock()
	if s.android.emulators == nil {
		s.android.emulators = map[string]*exec.Cmd{}
	}
	s.android.emulators[p.AVD] = cmd
	s.android.mu.Unlock()
	go cmd.Wait()
	return nil
}

func (s *Server) androidStop(p proto.AndroidDeviceParams) error {
	sdk, err := s.androidSDK()
	if err != nil {
		return err
	}
	if p.Serial == "" && p.AVD != "" {
		s.android.mu.Lock()
		cmd := s.android.emulators[p.AVD]
		s.android.mu.Unlock()
		if cmd != nil && cmd.Process != nil {
			return cmd.Process.Kill()
		}
		return fmt.Errorf("android: %s isn't running", p.AVD)
	}
	return sdk.Stop(p.Serial)
}

func (s *Server) androidConnect(p proto.AndroidConnectParams) error {
	sdk, err := s.androidSDK()
	if err != nil {
		return err
	}
	return sdk.Connect(strings.TrimSpace(p.Address), strings.TrimSpace(p.PairAddress), strings.TrimSpace(p.Code))
}

func (s *Server) androidLogcat(p proto.AndroidLogcatParams) (proto.AndroidLog, error) {
	sdk, err := s.androidSDK()
	if err != nil {
		return proto.AndroidLog{}, err
	}
	return s.android.logs.Read(sdk, p.Serial, p.Package, p.After), nil
}

// androidRun runs session p.Session's app on device p.Serial in a new
// tab: the project's command, with the SDK and the device in its
// environment, and the ADB server across the bridge when the device is
// on another host.
func (s *Server) androidRun(p proto.AndroidRunParams) (proto.AndroidRun, error) {
	sess, err := s.sessions.Get(p.Session)
	if err != nil {
		return proto.AndroidRun{}, err
	}
	dir := sess.Info().Dir
	proj := android.Project(dir)
	command := strings.TrimSpace(p.Command)
	if command == "" {
		command = proj.Command
	}
	if command == "" {
		return proto.AndroidRun{}, errors.New("android: this project isn't an app enowX knows how to run (Flutter, Expo, React Native, Capacitor, NativePHP, Gradle): type its command")
	}
	sdk, _ := android.FindSDK()
	env := append(sdk.Env(os.Getenv("PATH")), "ANDROID_SERIAL="+p.Serial)
	if p.ADB != "" {
		host, port, ok := strings.Cut(p.ADB, ":")
		if ok {
			env = append(env, "ADB_SERVER_SOCKET=tcp:"+p.ADB, "ANDROID_ADB_SERVER_ADDRESS="+host, "ANDROID_ADB_SERVER_PORT="+port)
		}
	}
	// The app on the phone reaches its dev server through the device's
	// localhost: Metro for React Native and Expo.
	if proj.Kind == "expo" || proj.Kind == "react-native" {
		command = `adb -s "$ANDROID_SERIAL" reverse tcp:8081 tcp:8081 >/dev/null 2>&1; ` + command
	}
	parent := sess.Meta().Parent
	if parent == "" {
		parent = sess.ID()
	}
	tab, err := s.createSession(proto.SessionCreateParams{
		Parent: parent, Name: "Android", Agent: agent.Custom, Command: thenShell(scriptArgv(command)), Env: env,
	})
	if err != nil {
		return proto.AndroidRun{}, err
	}
	return proto.AndroidRun{Tab: tab.ID(), Command: command, Package: proj.Package}, nil
}

// setupArgs is the command line setting Android up, a TV too when asked.
func setupArgs(exe string, tv bool) []string {
	if tv {
		return []string{exe, "android", "setup", "--tv"}
	}
	return []string{exe, "android", "setup"}
}
