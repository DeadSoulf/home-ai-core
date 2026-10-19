//go:build windows

package main

import (
	"context"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

type settingsCertificateProbeResult struct {
	server string
	info   windowsclient.ServerCertificateInfo
	err    error
}

func (state *windowsSettingsUI) refreshCertificatePinStatus() {
	if state.certificateStatus == 0 {
		return
	}
	rawServer := strings.TrimSpace(state.text(state.serverEdit))
	if rawServer == "" {
		state.setText(state.certificateStatus, state.tr("cert_pin_none"))
		state.setControlEnabled(state.certificateForgetBtn, false)
		return
	}
	pin, err := windowsclient.LoadServerCertificatePin(rawServer)
	if err != nil {
		state.setText(state.certificateStatus, state.tr("cert_pin_unavailable"))
		state.setControlEnabled(state.certificateForgetBtn, false)
		return
	}
	state.setControlEnabled(state.certificateForgetBtn, pin != "")
	if pin == "" {
		state.setText(state.certificateStatus, state.tr("cert_pin_none"))
	} else {
		state.setText(state.certificateStatus, state.trf("cert_pin_active", strings.ToUpper(pin[:20])))
	}
}

func (state *windowsSettingsUI) beginTrustServerCertificate() {
	if state.certificateProbeBusy {
		return
	}
	server := strings.TrimSpace(state.text(state.serverEdit))
	if server == "" {
		state.showError(state.tr("cert_server_required"))
		return
	}
	canonical, err := windowsclient.NormalizeServerURL(server)
	if err != nil {
		state.showTechnicalError(err)
		return
	}
	if !strings.HasPrefix(canonical, "https://") {
		state.showError(state.tr("cert_https_required"))
		return
	}
	state.certificateProbeBusy = true
	state.setControlEnabled(state.certificateTrustBtn, false)
	state.setText(state.certificateStatus, state.tr("cert_probe_running"))
	hwnd := uintptr(state.hwnd)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		info, probeErr := windowsclient.ProbeServerCertificate(ctx, canonical)
		state.asyncMu.Lock()
		state.certificateProbe = &settingsCertificateProbeResult{server: canonical, info: info, err: probeErr}
		state.asyncMu.Unlock()
		procSettingsPostMessage.Call(hwnd, settingsWMCertificateProbeDone, 0, 0)
	}()
}

func certificateFingerprintForDisplay(raw string) string {
	var bytes []string
	for i := 0; i+2 <= len(raw); i += 2 {
		bytes = append(bytes, strings.ToUpper(raw[i:i+2]))
	}
	return strings.Join(bytes, ":")
}

func (state *windowsSettingsUI) finishTrustServerCertificate() {
	state.asyncMu.Lock()
	result := state.certificateProbe
	state.certificateProbe = nil
	state.asyncMu.Unlock()
	if result == nil {
		return
	}
	state.certificateProbeBusy = false
	state.setControlEnabled(state.certificateTrustBtn, true)
	state.refreshCertificatePinStatus()
	if result.err != nil {
		state.showTechnicalError(result.err)
		return
	}
	current := strings.TrimSpace(state.text(state.serverEdit))
	canonical, err := windowsclient.NormalizeServerURL(current)
	if err != nil || canonical != result.server {
		state.showError(state.tr("cert_server_changed"))
		return
	}
	message := state.trf("cert_pin_confirmation", result.server,
		certificateFingerprintForDisplay(result.info.SHA256),
		result.info.Subject,
		result.info.Issuer,
		result.info.NotAfter.Local().Format("2006-01-02 15:04"))
	// The probe's fingerprint is untrusted. User confirmation must be based
	// on independent verification (direct console or trusted admin).
	if !state.confirm(message) {
		return
	}
	if err := windowsclient.SaveServerCertificatePin(result.server, result.info.SHA256); err != nil {
		state.showTechnicalError(err)
		return
	}
	state.refreshCertificatePinStatus()
	state.setStatus(state.tr("cert_pin_saved"))
	state.startConnect(false)
}

func (state *windowsSettingsUI) forgetServerCertificate() {
	server := strings.TrimSpace(state.text(state.serverEdit))
	pin, err := windowsclient.LoadServerCertificatePin(server)
	if err != nil {
		state.showTechnicalError(err)
		return
	}
	if pin == "" {
		state.refreshCertificatePinStatus()
		return
	}
	if !state.confirm(state.tr("cert_pin_forget_confirm")) {
		return
	}
	if err := windowsclient.RemoveServerCertificatePin(server); err != nil {
		state.showTechnicalError(err)
		return
	}
	state.connected = false
	state.refreshCertificatePinStatus()
	state.setStatus(state.tr("cert_pin_forgotten"))
	state.refreshOverview()
}
