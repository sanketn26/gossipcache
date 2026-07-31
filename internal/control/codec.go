package control

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
	case Hello:
		e.Uint64(m.NodeID)
		e.Uint16(uint16(m.Protocol.Version))
		e.Uint16(uint16(m.Protocol.MinSupported))
		e.Uint32(uint32(len(m.Subscriptions)))
		for _, subscription := range m.Subscriptions {
			e.Uint32(subscription.StreamID)
			e.Uint64(subscription.AppliedThrough)
			e.Uint64(subscription.HubGeneration)
		}
	case *Hello:
		return marshalPayloadV1(*m)
	case Subscribe:
		e.Uint64(m.HubGeneration)
		e.Uint32(uint32(len(m.StreamIDs)))
		for _, streamID := range m.StreamIDs {
			e.Uint32(streamID)
		}
	case *Subscribe:
		return marshalPayloadV1(*m)
	case InvalidationBatch:
		e.Uint32(m.StreamID)
		e.Uint64(m.HubGeneration)
		e.Uint32(uint32(len(m.Events)))
		for _, event := range m.Events {
			e.Uint64(event.StreamSequence)
			e.LengthBytes(event.Key)
			e.Uint32(event.Version.PartitionID)
			e.Uint64(event.Version.Sequence)
			e.Uint8(uint8(event.Kind))
			e.Raw(event.MutationID[:])
		}
	case *InvalidationBatch:
		return marshalPayloadV1(*m)
	case HopFrameAck:
		e.Uint32(m.StreamID)
		e.Uint64(m.ReceivedThrough)
	case *HopFrameAck:
		return marshalPayloadV1(*m)
	case StreamAcknowledgement:
		e.Uint32(m.StreamID)
		e.Uint64(m.AppliedThrough)
	case *StreamAcknowledgement:
		return marshalPayloadV1(*m)
	case StreamCheckpoint:
		e.Uint32(m.StreamID)
		e.Uint64(m.HubGeneration)
		e.Uint64(m.StreamHead)
	case *StreamCheckpoint:
		return marshalPayloadV1(*m)
	case ReplayRequest:
		e.Uint32(m.StreamID)
		e.Uint64(m.FromSequence)
		e.Uint64(m.ToSequence)
	case *ReplayRequest:
		return marshalPayloadV1(*m)
	case ReplayUnavailable:
		e.Uint32(m.StreamID)
		e.Uint64(m.HubGeneration)
		e.Uint64(m.RequestedFrom)
		e.Uint64(m.OldestAvailable)
		e.Uint64(m.StreamHead)
	case *ReplayUnavailable:
		return marshalPayloadV1(*m)
	case InvalidateConfirm:
		e.Uint32(m.StreamID)
		e.Uint64(m.StreamSequence)
		e.Uint64(m.NodeID)
	case *InvalidateConfirm:
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
	case MessageHello:
		message, err = decodeHello(d)
	case MessageSubscribe:
		message, err = decodeSubscribe(d)
	case MessageInvalidationBatch:
		message, err = decodeInvalidationBatch(d)
	case MessageHopFrameAck:
		var m HopFrameAck
		m.StreamID, err = d.Uint32()
		if err == nil {
			m.ReceivedThrough, err = d.Uint64()
		}
		message = m
	case MessageStreamAcknowledgement:
		var m StreamAcknowledgement
		m.StreamID, err = d.Uint32()
		if err == nil {
			m.AppliedThrough, err = d.Uint64()
		}
		message = m
	case MessageStreamCheckpoint:
		var m StreamCheckpoint
		m.StreamID, err = d.Uint32()
		if err == nil {
			m.HubGeneration, err = d.Uint64()
		}
		if err == nil {
			m.StreamHead, err = d.Uint64()
		}
		message = m
	case MessageReplayRequest:
		var m ReplayRequest
		m.StreamID, err = d.Uint32()
		if err == nil {
			m.FromSequence, err = d.Uint64()
		}
		if err == nil {
			m.ToSequence, err = d.Uint64()
		}
		message = m
	case MessageReplayUnavailable:
		var m ReplayUnavailable
		m.StreamID, err = d.Uint32()
		if err == nil {
			m.HubGeneration, err = d.Uint64()
		}
		if err == nil {
			m.RequestedFrom, err = d.Uint64()
		}
		if err == nil {
			m.OldestAvailable, err = d.Uint64()
		}
		if err == nil {
			m.StreamHead, err = d.Uint64()
		}
		message = m
	case MessageInvalidateConfirm:
		var m InvalidateConfirm
		m.StreamID, err = d.Uint32()
		if err == nil {
			m.StreamSequence, err = d.Uint64()
		}
		if err == nil {
			m.NodeID, err = d.Uint64()
		}
		message = m
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

func decodeHello(d *frame.Decoder) (Hello, error) {
	var m Hello
	var err error
	m.NodeID, err = d.Uint64()
	if err != nil {
		return m, err
	}
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
	count, err := d.Count(MaxSubscriptions, 20, ErrTooManySubscriptions)
	if err != nil {
		return m, err
	}
	m.Subscriptions = make([]StreamWatermark, count)
	for i := range m.Subscriptions {
		m.Subscriptions[i].StreamID, err = d.Uint32()
		if err == nil {
			m.Subscriptions[i].AppliedThrough, err = d.Uint64()
		}
		if err == nil {
			m.Subscriptions[i].HubGeneration, err = d.Uint64()
		}
		if err != nil {
			return m, err
		}
	}
	return m, nil
}

func decodeSubscribe(d *frame.Decoder) (Subscribe, error) {
	var m Subscribe
	var err error
	m.HubGeneration, err = d.Uint64()
	if err != nil {
		return m, err
	}
	count, err := d.Count(MaxSubscriptions, 4, ErrTooManySubscriptions)
	if err != nil {
		return m, err
	}
	m.StreamIDs = make([]uint32, count)
	for i := range m.StreamIDs {
		m.StreamIDs[i], err = d.Uint32()
		if err != nil {
			return m, err
		}
	}
	return m, nil
}

func decodeInvalidationBatch(d *frame.Decoder) (InvalidationBatch, error) {
	var m InvalidationBatch
	var err error
	m.StreamID, err = d.Uint32()
	if err == nil {
		m.HubGeneration, err = d.Uint64()
	}
	if err != nil {
		return m, err
	}
	count, err := d.Count(MaxBatchEvents, 41, ErrTooManyEvents)
	if err != nil {
		return m, err
	}
	m.Events = make([]InvalidationEvent, count)
	for i := range m.Events {
		event := &m.Events[i]
		event.StreamSequence, err = d.Uint64()
		if err == nil {
			event.Key, err = d.LengthBytes(wire.MaxKeyLen, wire.ErrKeyTooLarge)
		}
		if err == nil {
			event.Version.PartitionID, err = d.Uint32()
		}
		if err == nil {
			event.Version.Sequence, err = d.Uint64()
		}
		var kind uint8
		if err == nil {
			kind, err = d.Uint8()
			event.Kind = wire.RecordKind(kind)
		}
		if err == nil {
			var mutation []byte
			mutation, err = d.Take(len(event.MutationID))
			copy(event.MutationID[:], mutation)
		}
		if err != nil {
			return m, err
		}
	}
	return m, nil
}
