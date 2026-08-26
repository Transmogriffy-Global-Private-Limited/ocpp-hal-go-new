package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *PostgresStore) SyncV1Mapping(ctx context.Context, input V1MappingInput) (*V1ChargerMapping, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		if isDuplicate(err) {
			return nil, false, ErrV1MappingConflict
		}
		return nil, false, err
	}
	defer tx.Rollback()

	var existingIdentity string
	var existingCPO string
	var existingEnabled bool
	err = tx.QueryRowContext(ctx, `SELECT charger_ocpp_identity, cpo_id::text, enabled FROM v1_charger_mappings WHERE cms_charger_id = $1`, input.CMSChargerID).Scan(&existingIdentity, &existingCPO, &existingEnabled)
	created := errors.Is(err, sql.ErrNoRows)
	if err != nil && !created {
		return nil, false, err
	}
	if !created && (existingIdentity != input.ChargerOCPPIdentity || existingCPO != input.CPOID) {
		return nil, false, ErrV1MappingConflict
	}
	if created {
		_, err = tx.ExecContext(ctx, `INSERT INTO v1_charger_mappings (cms_charger_id, cpo_id, charger_ocpp_identity, expected_serial, enabled) VALUES ($1,$2,$3,NULLIF($4,''),$5)`, input.CMSChargerID, input.CPOID, input.ChargerOCPPIdentity, input.ExpectedSerial, input.Enabled)
		if err != nil {
			if isDuplicate(err) {
				return nil, false, ErrV1MappingConflict
			}
			return nil, false, err
		}
	}

	for _, connector := range input.Connectors {
		var oldCharger, oldCPO string
		var oldNumber int
		err = tx.QueryRowContext(ctx, `SELECT cms_charger_id::text, cpo_id::text, ocpp_connector_number FROM v1_connector_mappings WHERE cms_connector_id = $1`, connector.CMSConnectorID).Scan(&oldCharger, &oldCPO, &oldNumber)
		if err == nil {
			if oldCharger != input.CMSChargerID || oldCPO != input.CPOID || oldNumber != connector.OCPPConnectorNumber {
				return nil, false, ErrV1MappingConflict
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, false, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO v1_connector_mappings (cms_connector_id,cpo_id,cms_charger_id,charger_ocpp_identity,ocpp_connector_number) VALUES ($1,$2,$3,$4,$5)`, connector.CMSConnectorID, input.CPOID, input.CMSChargerID, input.ChargerOCPPIdentity, connector.OCPPConnectorNumber)
		if err != nil {
			if isDuplicate(err) {
				return nil, false, ErrV1MappingConflict
			}
			return nil, false, err
		}
	}

	if !created {
		_, err = tx.ExecContext(ctx, `UPDATE v1_charger_mappings SET enabled=$2, expected_serial=NULLIF($3,''), updated_at=NOW() WHERE cms_charger_id=$1`, input.CMSChargerID, input.Enabled, input.ExpectedSerial)
		if err != nil {
			return nil, false, err
		}
	}
	// Enrollment can occur after a charger has connected. Seed conservative
	// UNKNOWN runtime state so the mapping is queryable without inventing ONLINE.
	_, err = tx.ExecContext(ctx, `INSERT INTO v1_charger_runtime (charger_ocpp_identity, connection_state) VALUES ($1, 'UNKNOWN') ON CONFLICT (charger_ocpp_identity) DO NOTHING`, input.ChargerOCPPIdentity)
	if err != nil {
		if isDuplicate(err) {
			return nil, false, ErrV1IdempotencyConflict
		}
		return nil, false, err
	}
	action := "ENROLLED"
	if !created {
		action = "ENABLEMENT_CHANGED"
	}
	if created || existingEnabled != input.Enabled {
		auditID, idErr := NewSecureUUIDString()
		if idErr != nil {
			return nil, false, idErr
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO v1_mapping_audit (id,cpo_id,cms_charger_id,correlation_id,request_digest,action) VALUES ($1,$2,$3,$4,$5,$6)`, auditID, input.CPOID, input.CMSChargerID, input.CorrelationID, input.RequestDigest, action)
		if err != nil {
			return nil, false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	mapping, err := s.loadV1Mapping(ctx, input.CMSChargerID)
	return mapping, !created, err
}

func (s *PostgresStore) loadV1Mapping(ctx context.Context, cmsChargerID string) (*V1ChargerMapping, error) {
	m := &V1ChargerMapping{}
	err := s.db.QueryRowContext(ctx, `SELECT cpo_id::text,cms_charger_id::text,charger_ocpp_identity,COALESCE(expected_serial,''),enabled FROM v1_charger_mappings WHERE cms_charger_id=$1`, cmsChargerID).Scan(&m.CPOID, &m.CMSChargerID, &m.ChargerOCPPIdentity, &m.ExpectedSerial, &m.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrV1MappingNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT cpo_id::text,cms_charger_id::text,cms_connector_id::text,charger_ocpp_identity,ocpp_connector_number FROM v1_connector_mappings WHERE cms_charger_id=$1 ORDER BY ocpp_connector_number`, cmsChargerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c V1ConnectorMapping
		if err := rows.Scan(&c.CPOID, &c.CMSChargerID, &c.CMSConnectorID, &c.ChargerOCPPIdentity, &c.OCPPConnectorNumber); err != nil {
			return nil, err
		}
		m.Connectors = append(m.Connectors, c)
	}
	return m, rows.Err()
}

func (s *PostgresStore) ValidateV1Mapping(ctx context.Context, cpoID, cmsChargerID, cmsConnectorID, identity string, connector int) error {
	var enabled bool
	err := s.db.QueryRowContext(ctx, `SELECT m.enabled FROM v1_charger_mappings m JOIN v1_connector_mappings c ON c.cms_charger_id=m.cms_charger_id WHERE m.cpo_id=$1 AND m.cms_charger_id=$2 AND m.charger_ocpp_identity=$3 AND c.cms_connector_id=$4 AND c.charger_ocpp_identity=$3 AND c.ocpp_connector_number=$5`, cpoID, cmsChargerID, identity, cmsConnectorID, connector).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrV1MappingNotFound
	}
	if err != nil {
		return err
	}
	if !enabled {
		return ErrV1CredentialRejected
	}
	return nil
}

// ValidateV1ChargerAdmission is the connection-time guard. A mapped identity
// must also be enabled before it may create any runtime observation.
func (s *PostgresStore) ValidateV1ChargerAdmission(ctx context.Context, identity, presentedSerial string) error {
	var enabled bool
	var expectedSerial string
	err := s.db.QueryRowContext(ctx, `SELECT enabled,COALESCE(expected_serial,'') FROM v1_charger_mappings WHERE charger_ocpp_identity=$1`, identity).Scan(&enabled, &expectedSerial)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrV1MappingNotFound
	}
	if err != nil {
		return err
	}
	if !enabled {
		return ErrV1CredentialRejected
	}
	if expectedSerial != "" && presentedSerial != "" && expectedSerial != presentedSerial {
		return ErrV1IdentityConflict
	}
	return nil
}

func (s *PostgresStore) RecordV1BootEvidence(ctx context.Context, identity string, evidence V1BootEvidence) error {
	var expectedSerial string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(expected_serial,'') FROM v1_charger_mappings WHERE charger_ocpp_identity=$1`, identity).Scan(&expectedSerial)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrV1MappingNotFound
	}
	if err != nil {
		return err
	}
	serialConflict := expectedSerial != "" && evidence.PathSerial != "" && evidence.PathSerial != expectedSerial
	if !serialConflict && expectedSerial != "" && evidence.ChargePointSerialNumber != "" && evidence.ChargePointSerialNumber != expectedSerial {
		serialConflict = true
	}
	if !serialConflict && expectedSerial != "" && evidence.ChargeBoxSerialNumber != "" && evidence.ChargeBoxSerialNumber != expectedSerial {
		serialConflict = true
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO v1_charger_boot_evidence (charger_ocpp_identity,path_serial,charge_box_serial_number,charge_point_serial_number,charge_point_vendor,charge_point_model,firmware_version,serial_conflict,observed_at) VALUES ($1,NULLIF($2,''),NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9) ON CONFLICT (charger_ocpp_identity) DO UPDATE SET path_serial=EXCLUDED.path_serial,charge_box_serial_number=EXCLUDED.charge_box_serial_number,charge_point_serial_number=EXCLUDED.charge_point_serial_number,charge_point_vendor=EXCLUDED.charge_point_vendor,charge_point_model=EXCLUDED.charge_point_model,firmware_version=EXCLUDED.firmware_version,serial_conflict=EXCLUDED.serial_conflict,observed_at=EXCLUDED.observed_at`, identity, evidence.PathSerial, evidence.ChargeBoxSerialNumber, evidence.ChargePointSerialNumber, evidence.ChargePointVendor, evidence.ChargePointModel, evidence.FirmwareVersion, serialConflict, evidence.ObservedAt)
	return err
}

func (s *PostgresStore) CreateV1StartCommand(ctx context.Context, input V1StartCommandInput) (*V1RemoteCommand, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	commandID, err := NewSecureUUIDString()
	if err != nil {
		return nil, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO v1_remote_commands (id,cms_command_id,kind,request_digest,cpo_id,customer_id,correlation_id,cms_start_intent_id,cms_charger_id,cms_connector_id,charger_ocpp_identity,ocpp_connector_number,id_tag,credential_expires_at,command_expires_at,limit_type,energy_limit_wh,max_duration_seconds,state) VALUES ($1,$2,'START',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,'PERSISTED') ON CONFLICT (cms_command_id) DO NOTHING`, commandID, input.CMSCommandID, input.RequestDigest, input.CPOID, nullString(input.CustomerID), nullString(input.CorrelationID), input.CMSStartIntentID, input.CMSChargerID, input.CMSConnectorID, input.ChargerOCPPIdentity, input.OCPPConnectorNumber, input.IDTag, input.CredentialExpiresAt, input.CommandExpiresAt, input.LimitType, input.EnergyLimitWh, input.MaxDurationSeconds)
	if err != nil {
		if isDuplicate(err) {
			return nil, false, ErrV1IdempotencyConflict
		}
		return nil, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if rows == 0 {
		command, getErr := s.getV1CommandWith(ctx, tx, input.CMSCommandID)
		if getErr != nil {
			return nil, false, getErr
		}
		if command.RequestDigest != input.RequestDigest {
			return nil, false, ErrV1IdempotencyConflict
		}
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		return command, true, nil
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v1_start_credentials (id_tag,cms_start_intent_id,cpo_id,cms_charger_id,cms_connector_id,charger_ocpp_identity,ocpp_connector_number,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, input.IDTag, input.CMSStartIntentID, input.CPOID, input.CMSChargerID, input.CMSConnectorID, input.ChargerOCPPIdentity, input.OCPPConnectorNumber, input.CredentialExpiresAt)
	if err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	command, err := s.GetV1Command(ctx, input.CMSCommandID)
	return command, false, err
}

func (s *PostgresStore) CreateV1StopCommand(ctx context.Context, input V1StopCommandInput) (*V1RemoteCommand, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	transaction, err := s.getV1TransactionByID(ctx, txByQueryer{tx}, input.HALTransactionID)
	if err != nil {
		return nil, false, err
	}
	if transaction.CompletedAt != nil || transaction.OCPPTransactionID != input.OCPPTransactionID || transaction.CPOID != input.CPOID || transaction.CMSChargerID != input.CMSChargerID || transaction.CMSConnectorID != input.CMSConnectorID || transaction.ChargerOCPPIdentity != input.ChargerOCPPIdentity || transaction.OCPPConnectorNumber != input.OCPPConnectorNumber {
		return nil, false, ErrV1TransactionNotFound
	}
	workflow, _, err := s.ensureV1StopWorkflowTx(ctx, tx, transaction, input.RequestedStopInitiator, input.RequestedStopReason)
	if err != nil {
		return nil, false, err
	}
	commandID, err := NewSecureUUIDString()
	if err != nil {
		return nil, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO v1_remote_commands (id,cms_command_id,kind,request_digest,cpo_id,customer_id,correlation_id,cms_charging_session_id,cms_charger_id,cms_connector_id,charger_ocpp_identity,ocpp_connector_number,command_expires_at,requested_stop_initiator,requested_stop_reason,hal_transaction_id,ocpp_transaction_id,stop_workflow_transaction_id,state) VALUES ($1,$2,'STOP',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,'PERSISTED') ON CONFLICT (cms_command_id) DO NOTHING`, commandID, input.CMSCommandID, input.RequestDigest, input.CPOID, nullString(input.CustomerID), nullString(input.CorrelationID), input.CMSChargingSessionID, input.CMSChargerID, input.CMSConnectorID, input.ChargerOCPPIdentity, input.OCPPConnectorNumber, input.CommandExpiresAt, input.RequestedStopInitiator, input.RequestedStopReason, input.HALTransactionID, input.OCPPTransactionID, workflow.HALTransactionID)
	if err != nil {
		return nil, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if rows == 0 {
		command, err := s.getV1CommandWith(ctx, txByQueryer{tx}, input.CMSCommandID)
		if err != nil {
			return nil, false, err
		}
		if command.RequestDigest != input.RequestDigest {
			return nil, false, ErrV1IdempotencyConflict
		}
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		return command, true, nil
	}
	command, err := s.getV1CommandWith(ctx, txByQueryer{tx}, input.CMSCommandID)
	if err != nil {
		return nil, false, err
	}
	if err := s.insertV1FactTx(ctx, tx, "command.updated", command.HALCommandID, nil, time.Now().UTC(), v1CommandFact(command)); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return command, false, nil
}

func (s *PostgresStore) GetV1Command(ctx context.Context, cmsCommandID string) (*V1RemoteCommand, error) {
	return s.getV1CommandWith(ctx, s.db, cmsCommandID)
}

type v1Queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *PostgresStore) getV1CommandWith(ctx context.Context, q v1Queryer, cmsCommandID string) (*V1RemoteCommand, error) {
	c := &V1RemoteCommand{}
	var cred sql.NullTime
	var customer, correlation, startIntent, session, idtag, initiator, reason, hal, ocppID, result, category, detail sql.NullString
	var energy, duration sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT id::text,cms_command_id::text,kind,request_digest,cpo_id::text,customer_id::text,correlation_id,cms_start_intent_id::text,cms_charging_session_id::text,cms_charger_id::text,cms_connector_id::text,charger_ocpp_identity,ocpp_connector_number,id_tag,credential_expires_at,command_expires_at,limit_type,energy_limit_wh,max_duration_seconds,requested_stop_initiator,requested_stop_reason,hal_transaction_id::text,ocpp_transaction_id::text,state,delivery_attempts,last_ocpp_result,last_error_category,last_error_detail,created_at,updated_at FROM v1_remote_commands WHERE cms_command_id=$1`, cmsCommandID).Scan(&c.HALCommandID, &c.CMSCommandID, &c.Kind, &c.RequestDigest, &c.CPOID, &customer, &correlation, &startIntent, &session, &c.CMSChargerID, &c.CMSConnectorID, &c.ChargerOCPPIdentity, &c.OCPPConnectorNumber, &idtag, &cred, &c.CommandExpiresAt, &c.LimitType, &energy, &duration, &initiator, &reason, &hal, &ocppID, &c.State, &c.DeliveryAttempts, &result, &category, &detail, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrV1CommandNotFound
	}
	if err != nil {
		return nil, err
	}
	c.CustomerID = customer.String
	c.CorrelationID = correlation.String
	c.CMSStartIntentID = startIntent.String
	c.CMSChargingSessionID = session.String
	c.IDTag = idtag.String
	c.RequestedStopInitiator = initiator.String
	c.RequestedStopReason = reason.String
	c.HALTransactionID = hal.String
	c.LastOCPPResult = result.String
	c.LastErrorCategory = category.String
	c.LastErrorDetail = detail.String
	if cred.Valid {
		c.CredentialExpiresAt = &cred.Time
	}
	if energy.Valid {
		v := energy.Int64
		c.EnergyLimitWh = &v
	}
	if duration.Valid {
		v := duration.Int64
		c.MaxDurationSeconds = &v
	}
	if ocppID.Valid {
		var v int64
		_, err = fmt.Sscan(ocppID.String, &v)
		if err != nil {
			return nil, err
		}
		c.OCPPTransactionID = &v
	}
	return c, nil
}

func (s *PostgresStore) GetV1Credential(ctx context.Context, idTag string) (*V1Credential, error) {
	c := &V1Credential{}
	var consumed sql.NullTime
	var hal sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id_tag,cms_start_intent_id::text,cpo_id::text,cms_charger_id::text,cms_connector_id::text,charger_ocpp_identity,ocpp_connector_number,expires_at,consumed_at,hal_transaction_id::text FROM v1_start_credentials WHERE id_tag=$1`, idTag).Scan(&c.IDTag, &c.CMSStartIntentID, &c.CPOID, &c.CMSChargerID, &c.CMSConnectorID, &c.ChargerOCPPIdentity, &c.OCPPConnectorNumber, &c.ExpiresAt, &consumed, &hal)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrV1CredentialRejected
	}
	if err != nil {
		return nil, err
	}
	if consumed.Valid {
		c.ConsumedAt = &consumed.Time
	}
	c.HALTransactionID = hal.String
	return c, nil
}

func (s *PostgresStore) AuthorizeV1Credential(ctx context.Context, chargerID, idTag string, now time.Time) error {
	c, err := s.GetV1Credential(ctx, idTag)
	if err != nil {
		return err
	}
	if c.ChargerOCPPIdentity != chargerID || (!c.ExpiresAt.After(now) && c.HALTransactionID == "") {
		return ErrV1CredentialRejected
	}
	return nil
}

func (s *PostgresStore) MaterializeV1Start(ctx context.Context, input V1StartMaterialization) (*V1Transaction, bool, error) {
	if input.MeterStartWh < 0 || input.OCPPTransactionID <= 0 || input.OCPPConnectorNumber <= 0 || !plausibleV1ProtocolTime(input.ActualStartedAt, input.ObservedAt) {
		return nil, false, ErrV1InvalidEvidence
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var c V1Credential
	var consumed sql.NullTime
	var existing sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id_tag,cms_start_intent_id::text,cpo_id::text,cms_charger_id::text,cms_connector_id::text,charger_ocpp_identity,ocpp_connector_number,expires_at,consumed_at,hal_transaction_id::text FROM v1_start_credentials WHERE id_tag=$1 FOR UPDATE`, input.IDTag).Scan(&c.IDTag, &c.CMSStartIntentID, &c.CPOID, &c.CMSChargerID, &c.CMSConnectorID, &c.ChargerOCPPIdentity, &c.OCPPConnectorNumber, &c.ExpiresAt, &consumed, &existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrV1CredentialRejected
	}
	if err != nil {
		return nil, false, err
	}
	if existing.Valid {
		t, err := s.getV1TransactionByID(ctx, tx, existing.String)
		if err != nil {
			return nil, false, err
		}
		if t.ChargerOCPPIdentity == input.ChargerOCPPIdentity && t.OCPPConnectorNumber == input.OCPPConnectorNumber && t.IDTag == input.IDTag && t.MeterStartWh == input.MeterStartWh {
			return t, true, tx.Commit()
		}
		return nil, false, ErrV1CredentialRejected
	}
	if !c.ExpiresAt.After(input.ObservedAt) || c.ChargerOCPPIdentity != input.ChargerOCPPIdentity || c.OCPPConnectorNumber != input.OCPPConnectorNumber {
		return nil, false, ErrV1CredentialRejected
	}
	var command V1RemoteCommand
	var energy, duration sql.NullInt64
	var customer sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id::text,cms_command_id::text,command_expires_at,limit_type,energy_limit_wh,max_duration_seconds,customer_id::text FROM v1_remote_commands WHERE kind='START' AND id_tag=$1 FOR UPDATE`, input.IDTag).Scan(&command.HALCommandID, &command.CMSCommandID, &command.CommandExpiresAt, &command.LimitType, &energy, &duration, &customer)
	if err != nil {
		return nil, false, err
	}
	if energy.Valid {
		value := energy.Int64
		command.EnergyLimitWh = &value
	}
	if duration.Valid {
		value := duration.Int64
		command.MaxDurationSeconds = &value
	}
	command.CustomerID = customer.String
	if !command.CommandExpiresAt.After(input.ObservedAt) {
		return nil, false, ErrV1CredentialRejected
	}
	halID, err := NewSecureUUIDString()
	if err != nil {
		return nil, false, err
	}
	var deadline any = nil
	if command.MaxDurationSeconds != nil {
		deadline = input.ObservedAt.Add(time.Duration(*command.MaxDurationSeconds) * time.Second)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v1_transactions (hal_transaction_id,cms_start_intent_id,cms_command_id,cpo_id,customer_id,cms_charger_id,cms_connector_id,charger_ocpp_identity,ocpp_connector_number,id_tag,ocpp_transaction_id,actual_started_at,observed_started_at,meter_start_wh,limit_type,energy_limit_wh,max_duration_seconds,stop_deadline_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, halID, c.CMSStartIntentID, command.CMSCommandID, c.CPOID, nullString(command.CustomerID), c.CMSChargerID, c.CMSConnectorID, c.ChargerOCPPIdentity, c.OCPPConnectorNumber, c.IDTag, input.OCPPTransactionID, input.ActualStartedAt, input.ObservedAt, input.MeterStartWh, command.LimitType, command.EnergyLimitWh, command.MaxDurationSeconds, deadline)
	if err != nil {
		if isDuplicate(err) {
			return nil, false, ErrV1CredentialRejected
		}
		return nil, false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE v1_start_credentials SET consumed_at=$2,hal_transaction_id=$3 WHERE id_tag=$1`, input.IDTag, input.ObservedAt, halID)
	if err != nil {
		return nil, false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE v1_remote_commands SET state='MATERIALIZED',hal_transaction_id=$2,ocpp_transaction_id=$3,updated_at=NOW() WHERE cms_command_id=$1`, command.CMSCommandID, halID, input.OCPPTransactionID)
	if err != nil {
		return nil, false, err
	}
	materialized := &V1Transaction{HALTransactionID: halID, CMSStartIntentID: c.CMSStartIntentID, CMSCommandID: command.CMSCommandID, CPOID: c.CPOID, CustomerID: command.CustomerID, CMSChargerID: c.CMSChargerID, CMSConnectorID: c.CMSConnectorID, ChargerOCPPIdentity: c.ChargerOCPPIdentity, OCPPConnectorNumber: c.OCPPConnectorNumber, IDTag: c.IDTag, OCPPTransactionID: input.OCPPTransactionID, ActualStartedAt: input.ActualStartedAt, ObservedStartedAt: input.ObservedAt, MeterStartWh: input.MeterStartWh, LimitType: command.LimitType, EnergyLimitWh: command.EnergyLimitWh, MaxDurationSeconds: command.MaxDurationSeconds}
	if deadlineTime, ok := deadline.(time.Time); ok {
		materialized.StopDeadlineAt = &deadlineTime
	}
	if err := s.insertV1FactTx(ctx, tx, "transaction.started", halID, nil, input.ObservedAt, v1StartedFact(materialized, command.HALCommandID)); err != nil {
		return nil, false, err
	}
	materializedCommand := &V1RemoteCommand{HALCommandID: command.HALCommandID, CMSCommandID: command.CMSCommandID, Kind: "START", ChargerOCPPIdentity: c.ChargerOCPPIdentity, OCPPConnectorNumber: c.OCPPConnectorNumber, HALTransactionID: halID, OCPPTransactionID: &input.OCPPTransactionID, State: "MATERIALIZED", UpdatedAt: input.ObservedAt}
	if err := s.insertV1FactTx(ctx, tx, "command.updated", command.HALCommandID, nil, input.ObservedAt, v1CommandFact(materializedCommand)); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	out, err := s.GetV1Transaction(ctx, halID)
	return out, false, err
}

type txByQueryer struct{ *sql.Tx }

func (t txByQueryer) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, q, args...)
}
func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func (s *PostgresStore) GetV1Transaction(ctx context.Context, id string) (*V1Transaction, error) {
	return s.getV1TransactionByID(ctx, s.db, id)
}
func (s *PostgresStore) GetV1TransactionByStartIntent(ctx context.Context, id string) (*V1Transaction, error) {
	return s.getV1TransactionBy(ctx, s.db, `cms_start_intent_id=$1`, id)
}

type v1TransactionQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *PostgresStore) getV1TransactionByID(ctx context.Context, q v1TransactionQueryer, id string) (*V1Transaction, error) {
	return s.getV1TransactionBy(ctx, q, `hal_transaction_id=$1`, id)
}
func (s *PostgresStore) getV1TransactionBy(ctx context.Context, q v1TransactionQueryer, condition, arg string) (*V1Transaction, error) {
	t := &V1Transaction{}
	var customer, initiator, reason, ocppReason sql.NullString
	var latest, meterObs, deadline, completed, meterStop, observedStarted, observedCompleted sql.NullTime
	var latestVal, meterStopVal, rawMeterStopVal, adjustment, energy, duration sql.NullInt64
	var initialSoC, latestSoC sql.NullString
	var socObserved sql.NullTime
	err := q.QueryRowContext(ctx, `SELECT hal_transaction_id::text,cms_start_intent_id::text,cms_command_id::text,cpo_id::text,customer_id::text,cms_charger_id::text,cms_connector_id::text,charger_ocpp_identity,ocpp_connector_number,id_tag,ocpp_transaction_id,actual_started_at,observed_started_at,meter_start_wh,latest_meter_wh,meter_observed_at,meter_sequence,initial_soc_percent::text,latest_soc_percent::text,soc_observed_at,soc_sequence,limit_type,energy_limit_wh,max_duration_seconds,stop_deadline_at,stop_state,requested_stop_initiator,requested_stop_reason,ocpp_stop_reason,completed_at,observed_completed_at,meter_stop_wh,raw_meter_stop_wh,meter_stop_adjustment_wh,COALESCE(meter_stop_evidence,''),meter_quantization_anomaly_count FROM v1_transactions WHERE `+condition, arg).Scan(&t.HALTransactionID, &t.CMSStartIntentID, &t.CMSCommandID, &t.CPOID, &customer, &t.CMSChargerID, &t.CMSConnectorID, &t.ChargerOCPPIdentity, &t.OCPPConnectorNumber, &t.IDTag, &t.OCPPTransactionID, &t.ActualStartedAt, &observedStarted, &t.MeterStartWh, &latestVal, &meterObs, &t.MeterSequence, &initialSoC, &latestSoC, &socObserved, &t.SoCSequence, &t.LimitType, &energy, &duration, &deadline, &t.StopState, &initiator, &reason, &ocppReason, &completed, &observedCompleted, &meterStopVal, &rawMeterStopVal, &adjustment, &t.MeterStopEvidence, &t.MeterQuantizationAnomalyCount)
	_ = latest
	_ = meterStop
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrV1TransactionNotFound
	}
	if err != nil {
		return nil, err
	}
	t.CustomerID = customer.String
	t.RequestedStopInitiator = initiator.String
	t.RequestedStopReason = reason.String
	t.OCPPStopReason = ocppReason.String
	if observedStarted.Valid {
		t.ObservedStartedAt = observedStarted.Time
	}
	if latestVal.Valid {
		v := latestVal.Int64
		t.LatestMeterWh = &v
		if v >= t.MeterStartWh {
			consumed := v - t.MeterStartWh
			t.ConsumedWh = &consumed
		}
	}
	if meterObs.Valid {
		t.MeterObservedAt = &meterObs.Time
	}
	if initialSoC.Valid {
		if value, ok := ParseV1SoCPercent(initialSoC.String); ok {
			t.InitialSoCPercent = &value
		} else {
			return nil, ErrV1InvalidEvidence
		}
	}
	if latestSoC.Valid {
		if value, ok := ParseV1SoCPercent(latestSoC.String); ok {
			t.LatestSoCPercent = &value
		} else {
			return nil, ErrV1InvalidEvidence
		}
	}
	if socObserved.Valid {
		t.SoCObservedAt = &socObserved.Time
	}
	if energy.Valid {
		v := energy.Int64
		t.EnergyLimitWh = &v
	}
	if duration.Valid {
		v := duration.Int64
		t.MaxDurationSeconds = &v
	}
	if deadline.Valid {
		t.StopDeadlineAt = &deadline.Time
	}
	if completed.Valid {
		t.CompletedAt = &completed.Time
	}
	if observedCompleted.Valid {
		t.ObservedCompletedAt = &observedCompleted.Time
	}
	if meterStopVal.Valid {
		v := meterStopVal.Int64
		t.MeterStopWh = &v
	}
	if rawMeterStopVal.Valid {
		v := rawMeterStopVal.Int64
		t.RawMeterStopWh = &v
	}
	if adjustment.Valid {
		v := adjustment.Int64
		t.MeterStopAdjustmentWh = &v
	}
	return t, nil
}

func (s *PostgresStore) MarkV1CommandDelivery(ctx context.Context, id, status, result, detail string) (*V1RemoteCommand, error) {
	state := "DELIVERY_ATTEMPTED"
	category := ""
	if status == "Accepted" {
		state = "OCPP_ACCEPTED"
	}
	if status == "AMBIGUOUS" {
		state = "AMBIGUOUS"
		category = "delivery"
	}
	if status == "Rejected" {
		state = "OCPP_REJECTED"
		category = "ocpp"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	resultUpdate, err := tx.ExecContext(ctx, `UPDATE v1_remote_commands SET state=$2,last_ocpp_result=$3,last_error_category=$4,last_error_detail=$5,claimed_until=NULL,updated_at=NOW() WHERE cms_command_id=$1 AND state NOT IN ('MATERIALIZED','SUPERSEDED')`, id, state, nullString(result), nullString(category), nullString(detail))
	if err != nil {
		return nil, err
	}
	command, err := s.getV1CommandWith(ctx, txByQueryer{tx}, id)
	if err != nil {
		return nil, err
	}
	if rows, _ := resultUpdate.RowsAffected(); rows == 1 {
		if err := s.insertV1FactTx(ctx, tx, "command.updated", command.HALCommandID, nil, command.UpdatedAt, v1CommandFact(command)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return command, nil
}

func (s *PostgresStore) ClaimV1StartDelivery(ctx context.Context, cmsCommandID string) (*V1RemoteCommand, bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE v1_remote_commands SET state='PENDING_DELIVERY',claimed_until=NOW()+INTERVAL '30 seconds',updated_at=NOW() WHERE cms_command_id=$1 AND kind='START' AND state='PERSISTED'`, cmsCommandID)
	if err != nil {
		return nil, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	command, err := s.GetV1Command(ctx, cmsCommandID)
	return command, rows == 1, err
}

func (s *PostgresStore) BeginV1CommandDelivery(ctx context.Context, cmsCommandID string) (*V1RemoteCommand, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE v1_remote_commands SET state='DELIVERY_ATTEMPTED',delivery_attempts=delivery_attempts+1,claimed_until=NOW()+INTERVAL '2 minutes',updated_at=NOW() WHERE cms_command_id=$1 AND kind='START' AND state='PENDING_DELIVERY'`, cmsCommandID)
	if err != nil {
		return nil, err
	}
	return s.GetV1Command(ctx, cmsCommandID)
}

func (s *PostgresStore) RecordV1ChargerConnection(ctx context.Context, identity string, generation int64, online bool, at time.Time) error {
	state := "OFFLINE"
	if online {
		state = "ONLINE"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var cpoID, chargerID string
	var enabled bool
	err = tx.QueryRowContext(ctx, `SELECT cpo_id::text,cms_charger_id::text,enabled FROM v1_charger_mappings WHERE charger_ocpp_identity=$1 FOR SHARE`, identity).Scan(&cpoID, &chargerID, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrV1MappingNotFound
	}
	if err != nil {
		return err
	}
	if !enabled {
		return ErrV1CredentialRejected
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO v1_charger_runtime (charger_ocpp_identity,connection_state,connection_generation,connection_sequence,connected_at,last_observed_at,updated_at) VALUES ($1,$2,$3,1,CASE WHEN $2='ONLINE' THEN $4::timestamptz ELSE NULL END,$4::timestamptz,$4::timestamptz) ON CONFLICT (charger_ocpp_identity) DO UPDATE SET connection_state=EXCLUDED.connection_state,connection_generation=EXCLUDED.connection_generation,connection_sequence=v1_charger_runtime.connection_sequence+1,connected_at=CASE WHEN EXCLUDED.connection_state='ONLINE' THEN EXCLUDED.last_observed_at ELSE v1_charger_runtime.connected_at END,last_observed_at=EXCLUDED.last_observed_at,updated_at=EXCLUDED.updated_at WHERE EXCLUDED.connection_generation>v1_charger_runtime.connection_generation OR (EXCLUDED.connection_generation=v1_charger_runtime.connection_generation AND v1_charger_runtime.connection_state='ONLINE' AND EXCLUDED.connection_state='OFFLINE')`, identity, state, generation, at)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return tx.Commit()
	}
	var sequence int64
	if err := tx.QueryRowContext(ctx, `SELECT connection_sequence FROM v1_charger_runtime WHERE charger_ocpp_identity=$1`, identity).Scan(&sequence); err != nil {
		return err
	}
	if err := s.insertV1FactTx(ctx, tx, "charger.connection.updated", identity, &sequence, at, v1ConnectionFact(cpoID, chargerID, identity, state, generation, sequence, at)); err != nil {
		return err
	}
	return tx.Commit()
}

// RenewCurrentV1ChargerConnection records a current connection's durable
// liveness evidence. It cannot create or resurrect a connection state.
func (s *PostgresStore) RenewCurrentV1ChargerConnection(ctx context.Context, identity string, generation int64, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var cpoID, chargerID string
	var enabled bool
	err = tx.QueryRowContext(ctx, `SELECT cpo_id::text,cms_charger_id::text,enabled FROM v1_charger_mappings WHERE charger_ocpp_identity=$1 FOR SHARE`, identity).Scan(&cpoID, &chargerID, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrV1MappingNotFound
	}
	if err != nil {
		return err
	}
	if !enabled {
		return ErrV1CredentialRejected
	}

	var sequence int64
	err = tx.QueryRowContext(ctx, `UPDATE v1_charger_runtime SET last_observed_at=$3::timestamptz,updated_at=$3::timestamptz,connection_sequence=connection_sequence+1 WHERE charger_ocpp_identity=$1 AND connection_generation=$2 AND connection_state='ONLINE' AND last_observed_at<$3::timestamptz RETURNING connection_sequence`, identity, generation, at).Scan(&sequence)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if err := s.insertV1FactTx(ctx, tx, "charger.connection.updated", identity, &sequence, at, v1ConnectionFact(cpoID, chargerID, identity, "ONLINE", generation, sequence, at)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *PostgresStore) RecordV1ConnectorStatus(ctx context.Context, r V1ConnectorRuntime) error {
	if r.ObservedAt == nil {
		now := time.Now().UTC()
		r.ObservedAt = &now
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var enabled bool
	err = tx.QueryRowContext(ctx, `SELECT m.enabled FROM v1_charger_mappings m JOIN v1_connector_mappings c ON c.cms_charger_id=m.cms_charger_id WHERE c.charger_ocpp_identity=$1 AND c.ocpp_connector_number=$2 AND m.charger_ocpp_identity=$1`, r.ChargerOCPPIdentity, r.OCPPConnectorNumber).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrV1MappingNotFound
	}
	if err != nil {
		return err
	}
	if !enabled {
		return ErrV1CredentialRejected
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v1_connector_runtime (charger_ocpp_identity,ocpp_connector_number,status,error_code,info,vendor_id,vendor_error_code,observed_at,status_sequence,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,1,NOW()) ON CONFLICT (charger_ocpp_identity,ocpp_connector_number) DO UPDATE SET status=EXCLUDED.status,error_code=EXCLUDED.error_code,info=EXCLUDED.info,vendor_id=EXCLUDED.vendor_id,vendor_error_code=EXCLUDED.vendor_error_code,observed_at=EXCLUDED.observed_at,status_sequence=v1_connector_runtime.status_sequence+1,updated_at=NOW()`, r.ChargerOCPPIdentity, r.OCPPConnectorNumber, r.Status, nullString(r.ErrorCode), nullString(r.Info), nullString(r.VendorID), nullString(r.VendorErrorCode), r.ObservedAt)
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT cpo_id::text,cms_charger_id::text,cms_connector_id::text FROM v1_connector_mappings WHERE charger_ocpp_identity=$1 AND ocpp_connector_number=$2`, r.ChargerOCPPIdentity, r.OCPPConnectorNumber).Scan(&r.CPOID, &r.CMSChargerID, &r.CMSConnectorID)
	if err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT status_sequence FROM v1_connector_runtime WHERE charger_ocpp_identity=$1 AND ocpp_connector_number=$2`, r.ChargerOCPPIdentity, r.OCPPConnectorNumber).Scan(&r.StatusSequence); err != nil {
		return err
	}
	if err = s.insertV1FactTx(ctx, tx, "connector.status.updated", r.CMSConnectorID, &r.StatusSequence, *r.ObservedAt, v1StatusFact(&r)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *PostgresStore) GetV1ChargerRuntime(ctx context.Context, identity string) (*V1ChargerRuntime, error) {
	r := &V1ChargerRuntime{}
	var connected, observed sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT m.cpo_id::text,m.cms_charger_id::text,r.charger_ocpp_identity,r.connection_state,r.connection_generation,r.connection_sequence,r.connected_at,r.last_observed_at,r.updated_at FROM v1_charger_runtime r JOIN v1_charger_mappings m ON m.charger_ocpp_identity=r.charger_ocpp_identity WHERE r.charger_ocpp_identity=$1`, identity).Scan(&r.CPOID, &r.CMSChargerID, &r.ChargerOCPPIdentity, &r.ConnectionState, &r.ConnectionGeneration, &r.ConnectionSequence, &connected, &observed, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrV1MappingNotFound
	}
	if err != nil {
		return nil, err
	}
	if connected.Valid {
		r.ConnectedAt = &connected.Time
	}
	if observed.Valid {
		r.LastObservedAt = &observed.Time
	}
	rows, err := s.db.QueryContext(ctx, `SELECT m.cpo_id::text,m.cms_charger_id::text,c.cms_connector_id::text,c.charger_ocpp_identity,c.ocpp_connector_number,COALESCE(x.status,''),COALESCE(x.error_code,''),COALESCE(x.info,''),COALESCE(x.vendor_id,''),COALESCE(x.vendor_error_code,''),x.observed_at,COALESCE(x.status_sequence,0),COALESCE(x.updated_at,m.updated_at) FROM v1_connector_mappings c JOIN v1_charger_mappings m ON m.cms_charger_id=c.cms_charger_id LEFT JOIN v1_connector_runtime x ON x.charger_ocpp_identity=c.charger_ocpp_identity AND x.ocpp_connector_number=c.ocpp_connector_number WHERE c.charger_ocpp_identity=$1 ORDER BY c.ocpp_connector_number`, identity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c V1ConnectorRuntime
		var at sql.NullTime
		if err := rows.Scan(&c.CPOID, &c.CMSChargerID, &c.CMSConnectorID, &c.ChargerOCPPIdentity, &c.OCPPConnectorNumber, &c.Status, &c.ErrorCode, &c.Info, &c.VendorID, &c.VendorErrorCode, &at, &c.StatusSequence, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if at.Valid {
			c.ObservedAt = &at.Time
		}
		r.Connectors = append(r.Connectors, c)
	}
	return r, rows.Err()
}
func (s *PostgresStore) GetV1ConnectorRuntime(ctx context.Context, id string) (*V1ConnectorRuntime, error) {
	r := &V1ConnectorRuntime{}
	var at sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT m.cpo_id::text,m.cms_charger_id::text,c.cms_connector_id::text,c.charger_ocpp_identity,c.ocpp_connector_number,COALESCE(x.status,''),COALESCE(x.error_code,''),COALESCE(x.info,''),COALESCE(x.vendor_id,''),COALESCE(x.vendor_error_code,''),x.observed_at,COALESCE(x.status_sequence,0),COALESCE(x.updated_at,c.updated_at) FROM v1_connector_mappings c JOIN v1_charger_mappings m ON m.cms_charger_id=c.cms_charger_id LEFT JOIN v1_connector_runtime x ON x.charger_ocpp_identity=c.charger_ocpp_identity AND x.ocpp_connector_number=c.ocpp_connector_number WHERE c.cms_connector_id=$1`, id).Scan(&r.CPOID, &r.CMSChargerID, &r.CMSConnectorID, &r.ChargerOCPPIdentity, &r.OCPPConnectorNumber, &r.Status, &r.ErrorCode, &r.Info, &r.VendorID, &r.VendorErrorCode, &at, &r.StatusSequence, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrV1MappingNotFound
	}
	if err != nil {
		return nil, err
	}
	if at.Valid {
		r.ObservedAt = &at.Time
	}
	return r, nil
}
func (s *PostgresStore) ResetV1ConnectionRuntime(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Connection generations fence callbacks only within one live HAL process.
	// Startup has no surviving callbacks, so reset their durable comparison baseline
	// before recording the prior process's state as UNKNOWN.
	if _, err := tx.ExecContext(ctx, `UPDATE v1_charger_runtime r SET connection_generation=0 FROM v1_charger_mappings m WHERE r.charger_ocpp_identity=m.charger_ocpp_identity AND r.connection_generation<>0`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `UPDATE v1_charger_runtime r SET connection_state='UNKNOWN',connection_sequence=connection_sequence+1,updated_at=NOW() FROM v1_charger_mappings m WHERE r.charger_ocpp_identity=m.charger_ocpp_identity AND r.connection_state <> 'UNKNOWN' RETURNING m.cpo_id::text,m.cms_charger_id::text,r.charger_ocpp_identity,r.connection_generation,r.connection_sequence,r.updated_at`)
	if err != nil {
		return err
	}
	type resetRuntime struct {
		cpoID, chargerID, identity string
		generation, sequence       int64
		at                         time.Time
	}
	var changed []resetRuntime
	for rows.Next() {
		var item resetRuntime
		if err := rows.Scan(&item.cpoID, &item.chargerID, &item.identity, &item.generation, &item.sequence, &item.at); err != nil {
			rows.Close()
			return err
		}
		changed = append(changed, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range changed {
		if err := s.insertV1FactTx(ctx, tx, "charger.connection.updated", item.identity, &item.sequence, item.at, v1ConnectionFact(item.cpoID, item.chargerID, item.identity, "UNKNOWN", item.generation, item.sequence, item.at)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
