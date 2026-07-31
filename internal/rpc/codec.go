package rpc

import (
	"fmt"

	"github.com/sanketn26/gossipcache/internal/frame"
	"github.com/sanketn26/gossipcache/internal/wire"
)

func marshalPayloadVersion(version wire.ProtocolVersion, message Message) ([]byte, error) {
	switch version {
	case 1:
		return marshalPayloadV1(message)
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
}

func marshalPayloadV1(message Message) ([]byte, error) {
	var e frame.Encoder
	switch m := message.(type) {
	case HandshakeRequest:
		e.Uint16(uint16(m.Protocol.Version))
		e.Uint16(uint16(m.Protocol.MinSupported))
	case *HandshakeRequest:
		return marshalPayloadV1(*m)
	case Handshake:
		e.Uint16(uint16(m.ProtocolVersion))
		e.Uint64(m.HubGeneration)
		e.Uint32(m.PartitionCount)
		e.Uint8(uint8(m.StorageProfile))
		e.Bool(m.DurableHealthy)
	case *Handshake:
		return marshalPayloadV1(*m)
	case HubStatus:
		e.Uint64(m.HubGeneration)
		e.Uint8(uint8(m.StorageProfile))
		e.Bool(m.DurableHealthy)
	case *HubStatus:
		return marshalPayloadV1(*m)
	case GetRequest:
		e.LengthBytes(m.Key)
		if m.MinVersion != nil {
			e.Uint8(1)
			e.VersionTag(*m.MinVersion)
		} else {
			e.Uint8(0)
		}
	case *GetRequest:
		return marshalPayloadV1(*m)
	case GetResponse:
		e.Uint16(uint16(m.Status))
		e.Uint64(m.HubGeneration)
		e.VersionTag(m.Version)
		e.Uint8(uint8(m.Kind))
		e.Uint64(m.TTLMillis)
		e.LengthBytes(m.Value)
	case *GetResponse:
		return marshalPayloadV1(*m)
	case MutationRequest:
		e.Uint8(uint8(m.Op))
		e.LengthBytes(m.Key)
		e.LengthBytes(m.Value)
		e.Uint64(m.TTLMillis)
		e.Raw(m.MutationID[:])
		e.Uint8(uint8(m.Mode))
		e.Uint16(m.W)
		e.Uint8(uint8(m.Confirm))
		e.Uint32(m.Timeout)
	case *MutationRequest:
		return marshalPayloadV1(*m)
	case MutationResponse:
		e.Uint16(uint16(m.Status))
		e.Uint64(m.HubGeneration)
		e.VersionTag(m.Version)
	case *MutationResponse:
		return marshalPayloadV1(*m)
	default:
		return nil, fmt.Errorf("%w: %T", ErrInvalidMessage, message)
	}
	return e.Bytes(), nil
}

func unmarshalPayloadVersion(version wire.ProtocolVersion, messageType MessageType, payload []byte) (Message, error) {
	switch version {
	case 1:
		return unmarshalPayloadV1(messageType, payload)
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
}

func unmarshalPayloadV1(messageType MessageType, payload []byte) (Message, error) {
	d := frame.NewDecoder(payload).WithTruncated(ErrTruncatedFrame)
	var message Message
	var err error

	switch messageType {
	case MessageHandshakeRequest:
		message, err = decodeHandshakeRequest(d)
	case MessageHandshake:
		message, err = decodeHandshake(d)
	case MessageHubStatus:
		message, err = decodeHubStatus(d)
	case MessageGetRequest:
		message, err = decodeGetRequest(d)
	case MessageGetResponse:
		message, err = decodeGetResponse(d)
	case MessageMutationRequest:
		message, err = decodeMutationRequest(d)
	case MessageMutationResponse:
		message, err = decodeMutationResponse(d)
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownMessageType, messageType)
	}
	if err != nil {
		return nil, err
	}
	if d.Remaining() != 0 {
		return nil, ErrTrailingPayload
	}
	if err := message.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidMessage, err)
	}
	return message, nil
}

func decodeHandshakeRequest(d *frame.Decoder) (HandshakeRequest, error) {
	var m HandshakeRequest
	version, err := d.Uint16()
	if err != nil {
		return m, err
	}
	minimum, err := d.Uint16()
	if err != nil {
		return m, err
	}
	m.Protocol = wire.ProtocolRange{
		Version:      wire.ProtocolVersion(version),
		MinSupported: wire.ProtocolVersion(minimum),
	}
	return m, nil
}

func decodeHandshake(d *frame.Decoder) (Handshake, error) {
	var m Handshake
	version, err := d.Uint16()
	if err != nil {
		return m, err
	}
	m.ProtocolVersion = wire.ProtocolVersion(version)
	m.HubGeneration, err = d.Uint64()
	if err != nil {
		return m, err
	}
	m.PartitionCount, err = d.Uint32()
	if err != nil {
		return m, err
	}
	profile, err := d.Uint8()
	if err != nil {
		return m, err
	}
	m.StorageProfile = wire.StorageProfile(profile)
	m.DurableHealthy, err = d.Bool()
	return m, err
}

func decodeHubStatus(d *frame.Decoder) (HubStatus, error) {
	var m HubStatus
	var err error
	m.HubGeneration, err = d.Uint64()
	if err != nil {
		return m, err
	}
	profile, err := d.Uint8()
	if err != nil {
		return m, err
	}
	m.StorageProfile = wire.StorageProfile(profile)
	m.DurableHealthy, err = d.Bool()
	return m, err
}

func decodeGetRequest(d *frame.Decoder) (GetRequest, error) {
	var m GetRequest
	var err error
	m.Key, err = d.LengthBytes(wire.MaxKeyLen, wire.ErrKeyTooLarge)
	if err != nil {
		return m, err
	}
	flag, err := d.Uint8()
	if err != nil {
		return m, err
	}
	switch flag {
	case 0:
		return m, nil
	case 1:
		version, err := d.VersionTag()
		if err != nil {
			return m, err
		}
		m.MinVersion = &version
		return m, nil
	default:
		return m, fmt.Errorf("%w: min_version flag %d", ErrInvalidMessage, flag)
	}
}

func decodeGetResponse(d *frame.Decoder) (GetResponse, error) {
	var m GetResponse
	status, err := d.Uint16()
	if err != nil {
		return m, err
	}
	m.Status = wire.Status(status)
	m.HubGeneration, err = d.Uint64()
	if err != nil {
		return m, err
	}
	m.Version, err = d.VersionTag()
	if err != nil {
		return m, err
	}
	kind, err := d.Uint8()
	if err != nil {
		return m, err
	}
	m.Kind = wire.RecordKind(kind)
	m.TTLMillis, err = d.Uint64()
	if err != nil {
		return m, err
	}
	m.Value, err = d.LengthBytes(wire.MaxValueLen, wire.ErrValueTooLarge)
	return m, err
}

func decodeMutationRequest(d *frame.Decoder) (MutationRequest, error) {
	var m MutationRequest
	op, err := d.Uint8()
	if err != nil {
		return m, err
	}
	m.Op = Op(op)
	m.Key, err = d.LengthBytes(wire.MaxKeyLen, wire.ErrKeyTooLarge)
	if err != nil {
		return m, err
	}
	m.Value, err = d.LengthBytes(wire.MaxValueLen, wire.ErrValueTooLarge)
	if err != nil {
		return m, err
	}
	m.TTLMillis, err = d.Uint64()
	if err != nil {
		return m, err
	}
	mutation, err := d.Take(len(m.MutationID))
	if err != nil {
		return m, err
	}
	copy(m.MutationID[:], mutation)
	mode, err := d.Uint8()
	if err != nil {
		return m, err
	}
	m.Mode = wire.WriteMode(mode)
	m.W, err = d.Uint16()
	if err != nil {
		return m, err
	}
	confirm, err := d.Uint8()
	if err != nil {
		return m, err
	}
	m.Confirm = wire.ConfirmLevel(confirm)
	m.Timeout, err = d.Uint32()
	return m, err
}

func decodeMutationResponse(d *frame.Decoder) (MutationResponse, error) {
	var m MutationResponse
	status, err := d.Uint16()
	if err != nil {
		return m, err
	}
	m.Status = wire.Status(status)
	m.HubGeneration, err = d.Uint64()
	if err != nil {
		return m, err
	}
	m.Version, err = d.VersionTag()
	return m, err
}
