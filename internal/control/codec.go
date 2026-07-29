package control

import (
	"encoding/binary"
	"fmt"

	"github.com/sanketn26/gossipcache/internal/wire"
)

type encoder struct {
	bytes []byte
}

func (e *encoder) uint8(value uint8)   { e.bytes = append(e.bytes, value) }
func (e *encoder) uint16(value uint16) { e.appendFixed(2, uint64(value)) }
func (e *encoder) uint32(value uint32) { e.appendFixed(4, uint64(value)) }
func (e *encoder) uint64(value uint64) { e.appendFixed(8, value) }
func (e *encoder) raw(value []byte)    { e.bytes = append(e.bytes, value...) }
func (e *encoder) lengthBytes(value []byte) {
	e.uint32(uint32(len(value)))
	e.raw(value)
}

func (e *encoder) appendFixed(size int, value uint64) {
	start := len(e.bytes)
	e.bytes = append(e.bytes, make([]byte, size)...)
	switch size {
	case 2:
		binary.BigEndian.PutUint16(e.bytes[start:], uint16(value))
	case 4:
		binary.BigEndian.PutUint32(e.bytes[start:], uint32(value))
	case 8:
		binary.BigEndian.PutUint64(e.bytes[start:], value)
	}
}

func marshalPayloadVersion(version wire.ProtocolVersion, message Message) ([]byte, error) {
	switch version {
	case 1:
		return marshalPayloadV1(message)
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
	}
}

func marshalPayloadV1(message Message) ([]byte, error) {
	var e encoder
	switch m := message.(type) {
	case Hello:
		e.uint64(m.NodeID)
		e.uint16(uint16(m.Protocol.Version))
		e.uint16(uint16(m.Protocol.MinSupported))
		e.uint32(uint32(len(m.Subscriptions)))
		for _, subscription := range m.Subscriptions {
			e.uint32(subscription.StreamID)
			e.uint64(subscription.AppliedThrough)
			e.uint64(subscription.HubGeneration)
		}
	case *Hello:
		return marshalPayloadV1(*m)
	case Subscribe:
		e.uint64(m.HubGeneration)
		e.uint32(uint32(len(m.StreamIDs)))
		for _, streamID := range m.StreamIDs {
			e.uint32(streamID)
		}
	case *Subscribe:
		return marshalPayloadV1(*m)
	case InvalidationBatch:
		e.uint32(m.StreamID)
		e.uint64(m.HubGeneration)
		e.uint32(uint32(len(m.Events)))
		for _, event := range m.Events {
			e.uint64(event.StreamSequence)
			e.lengthBytes(event.Key)
			e.uint32(event.Version.PartitionID)
			e.uint64(event.Version.Sequence)
			e.uint8(uint8(event.Kind))
			e.raw(event.MutationID[:])
		}
	case *InvalidationBatch:
		return marshalPayloadV1(*m)
	case HopFrameAck:
		e.uint32(m.StreamID)
		e.uint64(m.ReceivedThrough)
	case *HopFrameAck:
		return marshalPayloadV1(*m)
	case StreamAcknowledgement:
		e.uint32(m.StreamID)
		e.uint64(m.AppliedThrough)
	case *StreamAcknowledgement:
		return marshalPayloadV1(*m)
	case StreamCheckpoint:
		e.uint32(m.StreamID)
		e.uint64(m.HubGeneration)
		e.uint64(m.StreamHead)
	case *StreamCheckpoint:
		return marshalPayloadV1(*m)
	case ReplayRequest:
		e.uint32(m.StreamID)
		e.uint64(m.FromSequence)
		e.uint64(m.ToSequence)
	case *ReplayRequest:
		return marshalPayloadV1(*m)
	case ReplayUnavailable:
		e.uint32(m.StreamID)
		e.uint64(m.HubGeneration)
		e.uint64(m.RequestedFrom)
		e.uint64(m.OldestAvailable)
		e.uint64(m.StreamHead)
	case *ReplayUnavailable:
		return marshalPayloadV1(*m)
	case InvalidateConfirm:
		e.uint32(m.StreamID)
		e.uint64(m.StreamSequence)
		e.uint64(m.NodeID)
	case *InvalidateConfirm:
		return marshalPayloadV1(*m)
	default:
		return nil, fmt.Errorf("%w: %T", ErrInvalidMessage, message)
	}
	return e.bytes, nil
}

type decoder struct {
	bytes  []byte
	offset int
}

func (d *decoder) remaining() int { return len(d.bytes) - d.offset }
func (d *decoder) take(size int) ([]byte, error) {
	if size < 0 || d.remaining() < size {
		return nil, ErrTruncatedFrame
	}
	value := d.bytes[d.offset : d.offset+size]
	d.offset += size
	return value, nil
}
func (d *decoder) uint8() (uint8, error) {
	value, err := d.take(1)
	if err != nil {
		return 0, err
	}
	return value[0], nil
}
func (d *decoder) uint16() (uint16, error) {
	value, err := d.take(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(value), nil
}
func (d *decoder) uint32() (uint32, error) {
	value, err := d.take(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(value), nil
}
func (d *decoder) uint64() (uint64, error) {
	value, err := d.take(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(value), nil
}
func (d *decoder) lengthBytes(max int) ([]byte, error) {
	length, err := d.uint32()
	if err != nil {
		return nil, err
	}
	if length > uint32(max) {
		return nil, wire.ErrKeyTooLarge
	}
	value, err := d.take(int(length))
	if err != nil {
		return nil, err
	}
	return wire.CopyBytes(value), nil
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
	d := decoder{bytes: payload}
	var message Message
	var err error

	switch messageType {
	case MessageHello:
		message, err = decodeHello(&d)
	case MessageSubscribe:
		message, err = decodeSubscribe(&d)
	case MessageInvalidationBatch:
		message, err = decodeInvalidationBatch(&d)
	case MessageHopFrameAck:
		var m HopFrameAck
		m.StreamID, err = d.uint32()
		if err == nil {
			m.ReceivedThrough, err = d.uint64()
		}
		message = m
	case MessageStreamAcknowledgement:
		var m StreamAcknowledgement
		m.StreamID, err = d.uint32()
		if err == nil {
			m.AppliedThrough, err = d.uint64()
		}
		message = m
	case MessageStreamCheckpoint:
		var m StreamCheckpoint
		m.StreamID, err = d.uint32()
		if err == nil {
			m.HubGeneration, err = d.uint64()
		}
		if err == nil {
			m.StreamHead, err = d.uint64()
		}
		message = m
	case MessageReplayRequest:
		var m ReplayRequest
		m.StreamID, err = d.uint32()
		if err == nil {
			m.FromSequence, err = d.uint64()
		}
		if err == nil {
			m.ToSequence, err = d.uint64()
		}
		message = m
	case MessageReplayUnavailable:
		var m ReplayUnavailable
		m.StreamID, err = d.uint32()
		if err == nil {
			m.HubGeneration, err = d.uint64()
		}
		if err == nil {
			m.RequestedFrom, err = d.uint64()
		}
		if err == nil {
			m.OldestAvailable, err = d.uint64()
		}
		if err == nil {
			m.StreamHead, err = d.uint64()
		}
		message = m
	case MessageInvalidateConfirm:
		var m InvalidateConfirm
		m.StreamID, err = d.uint32()
		if err == nil {
			m.StreamSequence, err = d.uint64()
		}
		if err == nil {
			m.NodeID, err = d.uint64()
		}
		message = m
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownMessageType, messageType)
	}
	if err != nil {
		return nil, err
	}
	if d.remaining() != 0 {
		return nil, ErrTrailingPayload
	}
	if err := message.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidMessage, err)
	}
	return message, nil
}

func decodeHello(d *decoder) (Hello, error) {
	var m Hello
	var err error
	m.NodeID, err = d.uint64()
	if err != nil {
		return m, err
	}
	version, err := d.uint16()
	if err != nil {
		return m, err
	}
	minimum, err := d.uint16()
	if err != nil {
		return m, err
	}
	m.Protocol = wire.ProtocolRange{
		Version:      wire.ProtocolVersion(version),
		MinSupported: wire.ProtocolVersion(minimum),
	}
	count, err := decodeCount(d, MaxSubscriptions, 20, ErrTooManySubscriptions)
	if err != nil {
		return m, err
	}
	m.Subscriptions = make([]StreamWatermark, count)
	for i := range m.Subscriptions {
		m.Subscriptions[i].StreamID, err = d.uint32()
		if err == nil {
			m.Subscriptions[i].AppliedThrough, err = d.uint64()
		}
		if err == nil {
			m.Subscriptions[i].HubGeneration, err = d.uint64()
		}
		if err != nil {
			return m, err
		}
	}
	return m, nil
}

func decodeSubscribe(d *decoder) (Subscribe, error) {
	var m Subscribe
	var err error
	m.HubGeneration, err = d.uint64()
	if err != nil {
		return m, err
	}
	count, err := decodeCount(d, MaxSubscriptions, 4, ErrTooManySubscriptions)
	if err != nil {
		return m, err
	}
	m.StreamIDs = make([]uint32, count)
	for i := range m.StreamIDs {
		m.StreamIDs[i], err = d.uint32()
		if err != nil {
			return m, err
		}
	}
	return m, nil
}

func decodeInvalidationBatch(d *decoder) (InvalidationBatch, error) {
	var m InvalidationBatch
	var err error
	m.StreamID, err = d.uint32()
	if err == nil {
		m.HubGeneration, err = d.uint64()
	}
	if err != nil {
		return m, err
	}
	count, err := decodeCount(d, MaxBatchEvents, 41, ErrTooManyEvents)
	if err != nil {
		return m, err
	}
	m.Events = make([]InvalidationEvent, count)
	for i := range m.Events {
		event := &m.Events[i]
		event.StreamSequence, err = d.uint64()
		if err == nil {
			event.Key, err = d.lengthBytes(wire.MaxKeyLen)
		}
		if err == nil {
			event.Version.PartitionID, err = d.uint32()
		}
		if err == nil {
			event.Version.Sequence, err = d.uint64()
		}
		var kind uint8
		if err == nil {
			kind, err = d.uint8()
			event.Kind = wire.RecordKind(kind)
		}
		if err == nil {
			var mutation []byte
			mutation, err = d.take(len(event.MutationID))
			copy(event.MutationID[:], mutation)
		}
		if err != nil {
			return m, err
		}
	}
	return m, nil
}

func decodeCount(d *decoder, maximum, minimumSize int, limitError error) (int, error) {
	count, err := d.uint32()
	if err != nil {
		return 0, err
	}
	if count > uint32(maximum) {
		return 0, limitError
	}
	if uint64(count)*uint64(minimumSize) > uint64(d.remaining()) {
		return 0, ErrTruncatedFrame
	}
	return int(count), nil
}
