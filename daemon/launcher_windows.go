package daemon

// placeLauncher has nothing to place: the daemon already runs from its own copy in
// the data folder (cmd/enowx's daemonExe), and agents' hooks don't run here.
func placeLauncher(dir, exe string) (string, error) { return exe, nil }
