package main

type agentTrayRuntime struct {
	RunNow <-chan struct{}
	Exit   <-chan struct{}
	close  func() error
}

func (tray *agentTrayRuntime) Close() error {
	if tray == nil || tray.close == nil {
		return nil
	}
	return tray.close()
}
