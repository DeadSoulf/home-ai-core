package main

type agentTrayRuntime struct {
	RunNow    <-chan struct{}
	UpdateNow <-chan struct{}
	Exit      <-chan struct{}
	setStatus func(string)
	notify    func(string, string, bool)
	close     func() error
}

func (tray *agentTrayRuntime) SetStatus(status string) {
	if tray != nil && tray.setStatus != nil {
		tray.setStatus(status)
	}
}

func (tray *agentTrayRuntime) Notify(title, message string, isError bool) {
	if tray != nil && tray.notify != nil {
		tray.notify(title, message, isError)
	}
}

func (tray *agentTrayRuntime) Close() error {
	if tray == nil || tray.close == nil {
		return nil
	}
	return tray.close()
}
