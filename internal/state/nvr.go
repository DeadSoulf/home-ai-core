package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNVRCameraNotFound = errors.New("NVR camera not found")

type NVRCameraRecord struct {
	ID            string
	Name          string
	Enabled       bool
	SourceType    string
	Address       string
	CredentialRef string
	Transport     string
	RecordingMode string
	AudioEnabled  bool
	CreatedBy     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type NVRONVIFSourceRecord struct {
	CameraID         string
	DeviceEndpoint   string
	MainProfileToken string
	SubProfileToken  string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type NVRStreamProfileRecord struct {
	ID         string
	CameraID   string
	Role       string
	SourceURI  string
	Codec      string
	Width      int
	Height     int
	FPS        float64
	BitrateBPS int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (s *Store) CreateNVRCamera(
	ctx context.Context,
	name, sourceType, address, credentialRef, transport, recordingMode, createdBy string,
	audioEnabled bool,
	now time.Time,
) (NVRCameraRecord, error) {
	name = strings.TrimSpace(name)
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))
	address = strings.TrimSpace(address)
	credentialRef = strings.TrimSpace(credentialRef)
	transport = strings.ToLower(strings.TrimSpace(transport))
	recordingMode = strings.ToLower(strings.TrimSpace(recordingMode))
	createdBy = strings.TrimSpace(createdBy)

	if name == "" {
		return NVRCameraRecord{}, errors.New("camera name is required")
	}
	if sourceType != "rtsp" && sourceType != "onvif" {
		return NVRCameraRecord{}, errors.New("camera source type must be rtsp or onvif")
	}
	if address == "" {
		return NVRCameraRecord{}, errors.New("camera address is required")
	}
	if credentialRef != "" && (!strings.HasPrefix(credentialRef, "sec_") || len(credentialRef) > 128) {
		return NVRCameraRecord{}, errors.New("camera credential reference is invalid")
	}
	if transport == "" {
		transport = "tcp"
	}
	if transport != "tcp" && transport != "udp" {
		return NVRCameraRecord{}, errors.New("camera transport must be tcp or udp")
	}
	if recordingMode == "" {
		recordingMode = "off"
	}
	switch recordingMode {
	case "off", "continuous", "motion":
	default:
		return NVRCameraRecord{}, errors.New("camera recording mode is invalid")
	}

	id, err := newStateID("cam_")
	if err != nil {
		return NVRCameraRecord{}, err
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	enabled := 1
	audio := 0
	if audioEnabled {
		audio = 1
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO nvr_cameras(
			id, name, enabled, source_type, address, credential_ref, transport,
			recording_mode, audio_enabled, created_by, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), ?, ?)
	`, id, name, enabled, sourceType, address, credentialRef, transport,
		recordingMode, audio, createdBy, timestamp, timestamp)
	if err != nil {
		return NVRCameraRecord{}, fmt.Errorf("create NVR camera: %w", err)
	}
	return s.NVRCamera(ctx, id)
}

func (s *Store) NVRCamera(ctx context.Context, cameraID string) (NVRCameraRecord, error) {
	var record NVRCameraRecord
	var enabled, audio int
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, enabled, source_type, address, COALESCE(credential_ref, ''),
		       transport, recording_mode, audio_enabled, COALESCE(created_by, ''),
		       created_at, updated_at
		FROM nvr_cameras
		WHERE id = ?
	`, strings.TrimSpace(cameraID)).Scan(
		&record.ID,
		&record.Name,
		&enabled,
		&record.SourceType,
		&record.Address,
		&record.CredentialRef,
		&record.Transport,
		&record.RecordingMode,
		&audio,
		&record.CreatedBy,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return NVRCameraRecord{}, ErrNVRCameraNotFound
	}
	if err != nil {
		return NVRCameraRecord{}, fmt.Errorf("read NVR camera: %w", err)
	}
	record.Enabled = enabled == 1
	record.AudioEnabled = audio == 1
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return NVRCameraRecord{}, fmt.Errorf("parse NVR camera created_at: %w", err)
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return NVRCameraRecord{}, fmt.Errorf("parse NVR camera updated_at: %w", err)
	}
	return record, nil
}

func (s *Store) UpdateNVRCamera(
	ctx context.Context,
	cameraID, name, sourceType, address, credentialRef, transport, recordingMode string,
	enabled, audioEnabled bool,
	now time.Time,
) (NVRCameraRecord, error) {
	cameraID = strings.TrimSpace(cameraID)
	name = strings.TrimSpace(name)
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))
	address = strings.TrimSpace(address)
	credentialRef = strings.TrimSpace(credentialRef)
	transport = strings.ToLower(strings.TrimSpace(transport))
	recordingMode = strings.ToLower(strings.TrimSpace(recordingMode))
	if cameraID == "" {
		return NVRCameraRecord{}, errors.New("camera id is required")
	}
	if name == "" {
		return NVRCameraRecord{}, errors.New("camera name is required")
	}
	if sourceType != "rtsp" && sourceType != "onvif" {
		return NVRCameraRecord{}, errors.New("camera source type must be rtsp or onvif")
	}
	if address == "" {
		return NVRCameraRecord{}, errors.New("camera address is required")
	}
	if credentialRef != "" && (!strings.HasPrefix(credentialRef, "sec_") || len(credentialRef) > 128) {
		return NVRCameraRecord{}, errors.New("camera credential reference is invalid")
	}
	if transport != "tcp" && transport != "udp" {
		return NVRCameraRecord{}, errors.New("camera transport must be tcp or udp")
	}
	switch recordingMode {
	case "off", "continuous", "motion":
	default:
		return NVRCameraRecord{}, errors.New("camera recording mode is invalid")
	}
	enabledValue := 0
	if enabled {
		enabledValue = 1
	}
	audioValue := 0
	if audioEnabled {
		audioValue = 1
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE nvr_cameras
		SET name = ?, enabled = ?, source_type = ?, address = ?, credential_ref = NULLIF(?, ''),
		    transport = ?, recording_mode = ?, audio_enabled = ?, updated_at = ?
		WHERE id = ?
	`, name, enabledValue, sourceType, address, credentialRef, transport, recordingMode,
		audioValue, now.UTC().Format(time.RFC3339Nano), cameraID)
	if err != nil {
		return NVRCameraRecord{}, fmt.Errorf("update NVR camera: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return NVRCameraRecord{}, fmt.Errorf("update NVR camera rows: %w", err)
	}
	if affected == 0 {
		return NVRCameraRecord{}, ErrNVRCameraNotFound
	}
	return s.NVRCamera(ctx, cameraID)
}

func (s *Store) DeleteNVRCamera(ctx context.Context, cameraID string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM nvr_cameras WHERE id = ?", strings.TrimSpace(cameraID))
	if err != nil {
		return fmt.Errorf("delete NVR camera: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete NVR camera rows: %w", err)
	}
	if affected == 0 {
		return ErrNVRCameraNotFound
	}
	return nil
}

func (s *Store) ListNVRCameras(ctx context.Context) ([]NVRCameraRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, enabled, source_type, address, COALESCE(credential_ref, ''),
		       transport, recording_mode, audio_enabled, COALESCE(created_by, ''),
		       created_at, updated_at
		FROM nvr_cameras
		ORDER BY name COLLATE NOCASE
	`)
	if err != nil {
		return nil, fmt.Errorf("list NVR cameras: %w", err)
	}
	defer rows.Close()

	result := make([]NVRCameraRecord, 0)
	for rows.Next() {
		var record NVRCameraRecord
		var enabled, audio int
		var createdAt, updatedAt string
		if err := rows.Scan(
			&record.ID,
			&record.Name,
			&enabled,
			&record.SourceType,
			&record.Address,
			&record.CredentialRef,
			&record.Transport,
			&record.RecordingMode,
			&audio,
			&record.CreatedBy,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan NVR camera: %w", err)
		}
		record.Enabled = enabled == 1
		record.AudioEnabled = audio == 1
		if record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parse NVR camera created_at: %w", err)
		}
		if record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, fmt.Errorf("parse NVR camera updated_at: %w", err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate NVR cameras: %w", err)
	}
	return result, nil
}

func (s *Store) SetNVRStreamProfile(
	ctx context.Context,
	cameraID, role, sourceURI, codec string,
	width, height int,
	fps float64,
	bitrateBPS int64,
	now time.Time,
) (NVRStreamProfileRecord, error) {
	cameraID = strings.TrimSpace(cameraID)
	role = strings.ToLower(strings.TrimSpace(role))
	sourceURI = strings.TrimSpace(sourceURI)
	codec = strings.TrimSpace(codec)
	if cameraID == "" || sourceURI == "" {
		return NVRStreamProfileRecord{}, errors.New("camera id and stream URI are required")
	}
	if role != "main" && role != "sub" {
		return NVRStreamProfileRecord{}, errors.New("stream role must be main or sub")
	}
	if width < 0 || height < 0 || fps < 0 || bitrateBPS < 0 {
		return NVRStreamProfileRecord{}, errors.New("stream metadata cannot be negative")
	}

	var id string
	err := s.db.QueryRowContext(ctx,
		"SELECT id FROM nvr_stream_profiles WHERE camera_id = ? AND role = ?",
		cameraID, role,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		id, err = newStateID("nvs_")
		if err != nil {
			return NVRStreamProfileRecord{}, err
		}
		timestamp := now.UTC().Format(time.RFC3339Nano)
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO nvr_stream_profiles(
				id, camera_id, role, source_uri, codec, width, height, fps, bitrate_bps,
				created_at, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, cameraID, role, sourceURI, codec, width, height, fps, bitrateBPS, timestamp, timestamp)
	} else if err == nil {
		_, err = s.db.ExecContext(ctx, `
			UPDATE nvr_stream_profiles
			SET source_uri = ?, codec = ?, width = ?, height = ?, fps = ?, bitrate_bps = ?, updated_at = ?
			WHERE id = ?
		`, sourceURI, codec, width, height, fps, bitrateBPS, now.UTC().Format(time.RFC3339Nano), id)
	}
	if err != nil {
		return NVRStreamProfileRecord{}, fmt.Errorf("set NVR stream profile: %w", err)
	}
	return s.nvrStreamProfile(ctx, id)
}

func (s *Store) DeleteNVRStreamProfile(ctx context.Context, cameraID, role string) error {
	cameraID = strings.TrimSpace(cameraID)
	role = strings.ToLower(strings.TrimSpace(role))
	if cameraID == "" {
		return errors.New("camera id is required")
	}
	if role != "main" && role != "sub" {
		return errors.New("stream role must be main or sub")
	}
	_, err := s.db.ExecContext(
		ctx,
		"DELETE FROM nvr_stream_profiles WHERE camera_id = ? AND role = ?",
		cameraID,
		role,
	)
	if err != nil {
		return fmt.Errorf("delete NVR stream profile: %w", err)
	}
	return nil
}

func (s *Store) ListNVRStreamProfiles(ctx context.Context, cameraID string) ([]NVRStreamProfileRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, camera_id, role, source_uri, codec, width, height, fps, bitrate_bps,
		       created_at, updated_at
		FROM nvr_stream_profiles
		WHERE camera_id = ?
		ORDER BY CASE role WHEN 'main' THEN 0 ELSE 1 END
	`, strings.TrimSpace(cameraID))
	if err != nil {
		return nil, fmt.Errorf("list NVR stream profiles: %w", err)
	}
	defer rows.Close()

	result := make([]NVRStreamProfileRecord, 0)
	for rows.Next() {
		var record NVRStreamProfileRecord
		var createdAt, updatedAt string
		if err := rows.Scan(
			&record.ID, &record.CameraID, &record.Role, &record.SourceURI, &record.Codec,
			&record.Width, &record.Height, &record.FPS, &record.BitrateBPS,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan NVR stream profile: %w", err)
		}
		record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse NVR stream profile created_at: %w", err)
		}
		record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse NVR stream profile updated_at: %w", err)
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *Store) nvrStreamProfile(ctx context.Context, id string) (NVRStreamProfileRecord, error) {
	var record NVRStreamProfileRecord
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, camera_id, role, source_uri, codec, width, height, fps, bitrate_bps,
		       created_at, updated_at
		FROM nvr_stream_profiles
		WHERE id = ?
	`, id).Scan(
		&record.ID, &record.CameraID, &record.Role, &record.SourceURI, &record.Codec,
		&record.Width, &record.Height, &record.FPS, &record.BitrateBPS,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return NVRStreamProfileRecord{}, err
	}
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return NVRStreamProfileRecord{}, err
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return NVRStreamProfileRecord{}, err
	}
	return record, nil
}


func (s *Store) SetNVRONVIFSource(
	ctx context.Context,
	cameraID, deviceEndpoint, mainProfileToken, subProfileToken string,
	now time.Time,
) (NVRONVIFSourceRecord, error) {
	cameraID = strings.TrimSpace(cameraID)
	deviceEndpoint = strings.TrimSpace(deviceEndpoint)
	mainProfileToken = strings.TrimSpace(mainProfileToken)
	subProfileToken = strings.TrimSpace(subProfileToken)
	if cameraID == "" || deviceEndpoint == "" || mainProfileToken == "" {
		return NVRONVIFSourceRecord{}, errors.New("camera id, ONVIF endpoint and main profile token are required")
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nvr_onvif_sources(
			camera_id, device_endpoint, main_profile_token, sub_profile_token, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(camera_id) DO UPDATE SET
			device_endpoint = excluded.device_endpoint,
			main_profile_token = excluded.main_profile_token,
			sub_profile_token = excluded.sub_profile_token,
			updated_at = excluded.updated_at
	`, cameraID, deviceEndpoint, mainProfileToken, subProfileToken, timestamp, timestamp)
	if err != nil {
		return NVRONVIFSourceRecord{}, fmt.Errorf("set NVR ONVIF source: %w", err)
	}
	return s.NVRONVIFSource(ctx, cameraID)
}

func (s *Store) NVRONVIFSource(ctx context.Context, cameraID string) (NVRONVIFSourceRecord, error) {
	var record NVRONVIFSourceRecord
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT camera_id, device_endpoint, main_profile_token, sub_profile_token, created_at, updated_at
		FROM nvr_onvif_sources
		WHERE camera_id = ?
	`, strings.TrimSpace(cameraID)).Scan(
		&record.CameraID,
		&record.DeviceEndpoint,
		&record.MainProfileToken,
		&record.SubProfileToken,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return NVRONVIFSourceRecord{}, err
	}
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return NVRONVIFSourceRecord{}, fmt.Errorf("parse NVR ONVIF source created_at: %w", err)
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return NVRONVIFSourceRecord{}, fmt.Errorf("parse NVR ONVIF source updated_at: %w", err)
	}
	return record, nil
}
