package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// ValidateV1ChargerOperationMapping protects the operation boundary from a
// CMS request that supplies a real OCPP identity but a mismatched CMS mapping.
// Connector zero is permitted only for whole-charge-point operations.
func (s *PostgresStore) ValidateV1ChargerOperationMapping(ctx context.Context, cpoID, cmsChargerID, cmsConnectorID, identity string, connector int) error {
	var enabled bool
	if connector == 0 {
		err := s.db.QueryRowContext(ctx, `SELECT enabled FROM v1_charger_mappings WHERE cpo_id=$1 AND cms_charger_id=$2 AND charger_ocpp_identity=$3`, cpoID, cmsChargerID, identity).Scan(&enabled)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrV1MappingNotFound
		}
		if err != nil {
			return err
		}
	} else {
		err := s.db.QueryRowContext(ctx, `SELECT m.enabled FROM v1_charger_mappings m JOIN v1_connector_mappings c ON c.cms_charger_id=m.cms_charger_id WHERE m.cpo_id=$1 AND m.cms_charger_id=$2 AND m.charger_ocpp_identity=$3 AND c.cms_connector_id=$4 AND c.charger_ocpp_identity=$3 AND c.ocpp_connector_number=$5`, cpoID, cmsChargerID, identity, cmsConnectorID, connector).Scan(&enabled)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrV1MappingNotFound
		}
		if err != nil {
			return err
		}
	}
	if !enabled {
		return ErrV1CredentialRejected
	}
	return nil
}

func (s *PostgresStore) CreateV1ChargerOperation(ctx context.Context, input V1ChargerOperationInput) (*V1ChargerOperation, bool, error) {
	id, err := NewSecureUUIDString()
	if err != nil {
		return nil, false, err
	}
	parameters, err := json.Marshal(input.Parameters)
	if err != nil {
		return nil, false, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO v1_charger_operations (id,cms_operation_id,request_digest,cpo_id,cms_charger_id,cms_connector_id,charger_ocpp_identity,ocpp_connector_number,kind,parameters,correlation_id,state) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8,$9,$10,$11,'PERSISTED') ON CONFLICT (cms_operation_id) DO NOTHING`, id, input.CMSOperationID, input.RequestDigest, input.CPOID, input.CMSChargerID, input.CMSConnectorID, input.ChargerOCPPIdentity, input.OCPPConnectorNumber, input.Kind, parameters, input.CorrelationID)
	if err != nil {
		return nil, false, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		existing, lookupErr := s.GetV1ChargerOperation(ctx, input.CMSOperationID)
		if lookupErr != nil {
			return nil, false, lookupErr
		}
		if existing.RequestDigest != input.RequestDigest {
			return nil, false, ErrV1IdempotencyConflict
		}
		return existing, true, nil
	}
	op, err := s.GetV1ChargerOperation(ctx, input.CMSOperationID)
	return op, false, err
}

func (s *PostgresStore) GetV1ChargerOperation(ctx context.Context, id string) (*V1ChargerOperation, error) {
	op := &V1ChargerOperation{}
	var connector sql.NullString
	var parameters []byte
	var completed sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT id::text,cms_operation_id::text,request_digest,cpo_id::text,cms_charger_id::text,cms_connector_id::text,charger_ocpp_identity,ocpp_connector_number,kind,parameters,correlation_id,state,delivery_attempts,COALESCE(ocpp_result,''),COALESCE(error_category,''),created_at,updated_at,completed_at FROM v1_charger_operations WHERE cms_operation_id=$1`, id).Scan(&op.HALOperationID, &op.CMSOperationID, &op.RequestDigest, &op.CPOID, &op.CMSChargerID, &connector, &op.ChargerOCPPIdentity, &op.OCPPConnectorNumber, &op.Kind, &parameters, &op.CorrelationID, &op.State, &op.DeliveryAttempts, &op.OCPPResult, &op.ErrorCategory, &op.CreatedAt, &op.UpdatedAt, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrV1OperationNotFound
	}
	if err != nil {
		return nil, err
	}
	op.CMSConnectorID = connector.String
	if err := json.Unmarshal(parameters, &op.Parameters); err != nil {
		return nil, err
	}
	if completed.Valid {
		at := completed.Time
		op.CompletedAt = &at
	}
	return op, nil
}

func (s *PostgresStore) ClaimV1ChargerOperationDelivery(ctx context.Context, id string) (*V1ChargerOperation, bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE v1_charger_operations SET state='DELIVERY_ATTEMPTED',delivery_attempts=delivery_attempts+1,updated_at=NOW() WHERE cms_operation_id=$1 AND state='PERSISTED'`, id)
	if err != nil {
		return nil, false, err
	}
	op, err := s.GetV1ChargerOperation(ctx, id)
	if err != nil {
		return nil, false, err
	}
	rows, _ := result.RowsAffected()
	return op, rows == 1, nil
}

func (s *PostgresStore) MarkV1ChargerOperationDelivery(ctx context.Context, id, state, result, category string) (*V1ChargerOperation, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE v1_charger_operations SET state=$2,ocpp_result=NULLIF($3,''),error_category=NULLIF($4,''),completed_at=NOW(),updated_at=NOW() WHERE cms_operation_id=$1 AND state='DELIVERY_ATTEMPTED'`, id, state, result, category)
	if err != nil {
		return nil, err
	}
	return s.GetV1ChargerOperation(ctx, id)
}
