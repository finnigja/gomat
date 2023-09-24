package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
)

const TYPE_UINT_1 = 4
const TYPE_UINT_2 = 5
const TYPE_UINT_4 = 6
const TYPE_UINT_8 = 7


const PROTOCOL_ID_SECURE_CHANNEL = 0

const SEC_CHAN_OPCODE_ACK        = 0x10
const SEC_CHAN_OPCODE_PBKDF_REQ  = 0x20
const SEC_CHAN_OPCODE_PBKDF_RESP = 0x21
const SEC_CHAN_OPCODE_PAKE1      = 0x22
const SEC_CHAN_OPCODE_PAKE2      = 0x23
const SEC_CHAN_OPCODE_PAKE3      = 0x24
const SEC_CHAN_OPCODE_STATUS_REP = 0x40

type TLVBuffer struct {
	data bytes.Buffer
}

type TLVBufferDec struct {
	data *bytes.Buffer
}

func (b *TLVBufferDec) checkAndSkip(d byte) error {
	o, err := b.data.ReadByte()
	if err != nil {
		return err
	}
	if o != d {
		b.data.UnreadByte()
		return fmt.Errorf("unexpected byte %x, expected %x", o, d)
	}
	return nil
}

func (b *TLVBufferDec) checkAndSkipBytes(d []byte) error {
	did_read := 0
	for _, now := range d {
		o, err := b.data.ReadByte()
		if err != nil {
			return err
		}
		did_read = did_read + 1
		if o != now {
			for i:=0; i<did_read; i++ {
				b.data.UnreadByte()
			}
			return fmt.Errorf("unexpected byte %x, expected %x", o, d)
		}
	}
	return nil
}

func (b *TLVBufferDec) readOctetString(itag byte) ([]byte, error) {
	ctrl, err := b.data.ReadByte()
	if err != nil {
		return nil, err
	}
	tagtype := ctrl >>5
	if tagtype != 1 {
		return nil, fmt.Errorf("can't handle tag type %x", ctrl)
	}
	tag, err := b.data.ReadByte()
	if err != nil {
		return nil, err
	}
	if tag != itag {
		return nil, fmt.Errorf("unexpected tag %d, expected %d", tag, itag)
	}
	tp := ctrl & 0x1f
	if tp != 0x10 {
		return nil, fmt.Errorf("can't handle octet string type %x", ctrl)
	}
	s, err := b.data.ReadByte()
	if err != nil {
		return nil, err
	}
	out := make([]byte, s)
	n, err := b.data.Read(out)
	if err != nil {
		return nil, err
	}
	if n != int(s) {
		return nil, fmt.Errorf("not able to read %d bytes", s)
	}
	return out, nil
}

func (b *TLVBufferDec) readUInt(itag byte) (uint64, error) {
	ctrl, err := b.data.ReadByte()
	if err != nil {
		return 0, err
	}
	tagtype := ctrl >>5
	if tagtype != 1 {
		return 0, fmt.Errorf("can't handle tag type %x", ctrl)
	}
	tag, err := b.data.ReadByte()
	if err != nil {
		return 0, err
	}
	if tag != itag {
		return 0, fmt.Errorf("unexpected tag %d, expected %d", tag, itag)
	}
	tp := ctrl & 0x1f
	if tp == TYPE_UINT_1 {
		o, err := b.data.ReadByte()
		if err != nil {
			return 0, err
		}
		return uint64(o), nil

	}
	if tp == TYPE_UINT_2 {
		var o uint16
		binary.Read(b.data, binary.LittleEndian, &o)
		if err != nil {
			return 0, err
		}
		return uint64(o), nil

	}
	if tp == TYPE_UINT_4 {
		var o uint32
		binary.Read(b.data, binary.LittleEndian, &o)
		if err != nil {
			return 0, err
		}
		return uint64(o), nil

	}
	if tp == TYPE_UINT_8 {
		var o uint64
		binary.Read(b.data, binary.LittleEndian, &o)
		if err != nil {
			return 0, err
		}
		return uint64(o), nil

	}
	return 0, fmt.Errorf("can't handle uint string type %x", ctrl)
}



func (b *TLVBuffer) writeControl(ctrl byte) {
	binary.Write(&b.data, binary.BigEndian, ctrl)
}

func (b *TLVBuffer) writeTagContentSpecific(tag byte) {
	binary.Write(&b.data, binary.BigEndian, tag)
}

func (b *TLVBuffer) writeUInt(tag byte, typ int, val uint64) {
	var ctrl byte
	ctrl = 0x1 << 5
	ctrl = ctrl | byte(typ)
	b.data.WriteByte(ctrl)
	b.data.WriteByte(tag)
	switch typ {
	case TYPE_UINT_1: b.data.WriteByte(byte(val))
	case TYPE_UINT_2: binary.Write(&b.data, binary.LittleEndian, uint16(val))
	case TYPE_UINT_4: binary.Write(&b.data, binary.LittleEndian, uint32(val))
	case TYPE_UINT_8: binary.Write(&b.data, binary.LittleEndian, uint64(val))
	}
}

func (b *TLVBuffer) writeOctetString(tag byte, data []byte) {
	var ctrl byte
	ctrl = 0x1 << 5
	ctrl = ctrl | 0x10
	b.data.WriteByte(ctrl)
	b.data.WriteByte(tag)
	b.data.WriteByte(byte(len(data)))
	b.data.Write(data)
}

func (b *TLVBuffer) writeBool(tag byte, val bool) {
	var ctrl byte
	ctrl = 0x1 << 5
	if val {
		ctrl = ctrl | 0x8
	} else {
		ctrl = ctrl | 0x9
	}
	b.data.WriteByte(ctrl)
	b.data.WriteByte(tag)
}

func (b *TLVBuffer) writeAnonStruct() {
	b.data.WriteByte(0x15)
}
func (b *TLVBuffer) writeAnonStructEnd() {
	b.data.WriteByte(0x18)
}


type ProtocolMessage struct {
	exchangeFlags byte
	opcode byte
	exchangeId uint16
	protocolId uint16
	ackCounter uint32
}

type Message struct {
	sessionId uint16
	securityFlags byte
	messageCounter uint32
	sourceNodeId []byte
	destinationNodeId []byte
	prot ProtocolMessage
}

func (m *Message)dump()  {
	fmt.Printf("  sessionId  : %d\n", m.sessionId)
	fmt.Printf("  secFlags   : %d\n", m.securityFlags)
	fmt.Printf("  msgCounter : %d\n", m.messageCounter)
	fmt.Printf("  srcNode    : %v\n", m.sourceNodeId)
	fmt.Printf("  dstNode    : %v\n", m.destinationNodeId)
	fmt.Printf("  prot       :\n")
	fmt.Printf("    exchangeFlags : %d\n", m.prot.exchangeFlags)
	fmt.Printf("    opcode        : 0x%x\n", m.prot.opcode)
	fmt.Printf("    exchangeId    : %d\n", m.prot.exchangeId)
	fmt.Printf("    protocolId    : %d\n", m.prot.protocolId)
	fmt.Printf("    ackCounter    : %d\n", m.prot.ackCounter)
}


func (m *Message)calcMessageFlags() byte {
	var out byte
	out = 0 // version hardcoded = 0

	if len(m.sourceNodeId) == 8 {
		out = out | 4
	}

	dsiz := 0
	if len(m.destinationNodeId) == 2 {
		dsiz = 2
	} else if len(m.destinationNodeId) == 8 {
		dsiz = 1
	}

	out = out | byte(dsiz)
	return out
}

func (m *Message) encodeBase(data *bytes.Buffer) {
	data.WriteByte(m.calcMessageFlags())
	binary.Write(data, binary.LittleEndian, uint16(m.sessionId))
	data.WriteByte(m.securityFlags)
	binary.Write(data, binary.LittleEndian, uint32(m.messageCounter))
	if len(m.sourceNodeId) == 8 {
		data.Write(m.sourceNodeId)
	}
	if len(m.destinationNodeId) > 0 {
		data.Write(m.destinationNodeId)
	}
}

func (m *Message) encode(data *bytes.Buffer) {
	data.WriteByte(m.calcMessageFlags())
	binary.Write(data, binary.LittleEndian, uint16(m.sessionId))
	data.WriteByte(m.securityFlags)
	binary.Write(data, binary.LittleEndian, uint32(m.messageCounter))
	if len(m.sourceNodeId) == 8 {
		data.Write(m.sourceNodeId)
	}
	if len(m.destinationNodeId) > 0 {
		data.Write(m.destinationNodeId)
	}

	data.WriteByte(m.prot.exchangeFlags)
	data.WriteByte(m.prot.opcode)
	binary.Write(data, binary.LittleEndian, uint16(m.prot.exchangeId))
	binary.Write(data, binary.LittleEndian, uint16(m.prot.protocolId))
}

func (m *Message) decode(data *bytes.Buffer) error {
	flags, err := data.ReadByte()
	if err != nil {
		return err
	}
	binary.Read(data, binary.LittleEndian, &m.sessionId)
	m.securityFlags, err = data.ReadByte()
	if err != nil {
		return err
	}
	binary.Read(data, binary.LittleEndian, &m.messageCounter)
	if (flags & 4) != 0 {
		m.sourceNodeId = make([]byte, 8)
		_, err := data.Read(m.sourceNodeId)
		if err != nil {
			return err
		}
	}
	if ((flags & 3 )!= 0) {
		dsiz := 0
		if (flags & 3) == 1 {
			dsiz = 8
		} else if (flags & 3) == 2 {
			dsiz = 2
		}
		if dsiz != 0 {
			m.destinationNodeId = make([]byte, dsiz)
			_, err := data.Read(m.destinationNodeId)
			if err != nil {
				return err
			}
		}
	}
	m.prot.exchangeFlags, err = data.ReadByte()
	if err != nil {
		return err
	}
	m.prot.opcode, err = data.ReadByte()
	if err != nil {
		return err
	}
	binary.Read(data, binary.LittleEndian, &m.prot.exchangeId)
	binary.Read(data, binary.LittleEndian, &m.prot.protocolId)
	if (m.prot.exchangeFlags & 0x2) != 0 {
		binary.Read(data, binary.LittleEndian, &m.prot.ackCounter)
	}
	return nil
}

/*
func test1() {
	msg := Message {
		sessionId: 0x1234,
		securityFlags: 0,
		messageCounter: 1,
		sourceNodeId: []byte{1,2,3,4,5,6,7,8},
		prot: ProtocolMessage{
			exchangeFlags: 5,
			opcode: 0x20,
			exchangeId: 0xba3e,
			protocolId: PROTOCOL_ID_SECURE_CHANNEL,
		},
	}
	var buffer bytes.Buffer
	msg.encode(&buffer)
	out := buffer.Bytes()
	dbg := hex.EncodeToString(out)
	log.Println(dbg)
	buf := bytes.NewBuffer(out)
	var msgdec Message
	err := msgdec.decode(buf)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v", msgdec)
	msgdec.dump()
}
*/
func test2() {
	var tlv TLVBuffer
	tlv.writeAnonStruct()
	bytes, err := hex.DecodeString("bbcbd707308cb511a5b7909ee2e15eeeed2a24372f851499b2d0dfc9485eae8f")
	if err != nil {
		panic(err)
	}
	tlv.writeOctetString(0x1, bytes)
	tlv.writeUInt(0x2, TYPE_UINT_2, 0xfdb8)
	tlv.writeUInt(0x3, TYPE_UINT_1, 0x00)
	tlv.writeBool(0x4, true)
	tlv.writeAnonStructEnd()
	dbg := hex.EncodeToString(tlv.data.Bytes())
	log.Println(dbg)
}

func PBKDFParamRequest() []byte {
	var buffer bytes.Buffer
	msg := Message {
		sessionId: 0x0,
		securityFlags: 0,
		messageCounter: 1,
		sourceNodeId: []byte{1,2,3,4,5,6,7,8},
		prot: ProtocolMessage{
			exchangeFlags: 5,
			opcode: SEC_CHAN_OPCODE_PBKDF_REQ,
			exchangeId: 0xba3e,
			protocolId: 0x00,
		},
	}
	msg.encode(&buffer)
	log.Printf("SIZZZZZ %d\n", len(buffer.Bytes()))

	var tlv TLVBuffer
	tlv.writeAnonStruct()
	bytes, err := hex.DecodeString("bbcbd707308cb511a5b7909ee2e15eeeed2a24372f851499b2d0dfc9485eae8f")
	if err != nil {
		panic(err)
	}
	tlv.writeOctetString(0x1, bytes)             // initiator random
	tlv.writeUInt(0x2, TYPE_UINT_2, 0x0001)      //initator session-id
	tlv.writeUInt(0x3, TYPE_UINT_1, 0x00)        // passcode id
	tlv.writeBool(0x4, true)                     // has pbkdf
	tlv.writeAnonStructEnd()
	buffer.Write(tlv.data.Bytes())
	return buffer.Bytes()
}

type PBKDFParamResponse struct {
	initiatorRandom []byte
	responderRandom []byte
	responderSession int
	iterations int
	salt []byte
}
type PAKE2ParamResponse struct {
	pb []byte
	cb []byte
}

type StatusReport struct {
	generalCode uint16
	protocolId uint32
	protocolCode uint16
}
func (d StatusReport)dump() {
	fmt.Printf(" generalCode  : %d\n", d.generalCode)
	fmt.Printf(" protocolId   : %d\n", d.protocolId)
	fmt.Printf(" protocolCode : %d\n", d.protocolCode)
}

type AllResp struct {
	messageCounter uint32
	PBKDFParamResponse *PBKDFParamResponse
	PAKE2ParamResponse *PAKE2ParamResponse
	StatusReport StatusReport
}

func (d PBKDFParamResponse)dump() {
	fmt.Printf(" initiatorRandom : %s\n", hex.EncodeToString(d.initiatorRandom))
	fmt.Printf(" responderRandom : %s\n", hex.EncodeToString(d.responderRandom))
	fmt.Printf(" responderSession: %d\n", d.responderSession)
	fmt.Printf(" iterations      : %d\n", d.iterations)
	fmt.Printf(" salt            : %s\n", hex.EncodeToString(d.salt))
}

func decodeStatusReport(buf *bytes.Buffer) AllResp {
	log.Printf("status report data %s", hex.EncodeToString(buf.Bytes()))
	var StatusReport StatusReport
	binary.Read(buf, binary.LittleEndian, &StatusReport.generalCode)
	binary.Read(buf, binary.LittleEndian, &StatusReport.protocolId)
	binary.Read(buf, binary.LittleEndian, &StatusReport.protocolCode)

	return AllResp{
		StatusReport: StatusReport,
	}
}

func decodePBKDFParamResponse(buf *bytes.Buffer) AllResp {
	var out PBKDFParamResponse
	var tlv TLVBufferDec
	tlv.data = buf
	err := tlv.checkAndSkip(0x15)
	if err != nil {
		panic(err)
	}
	out.initiatorRandom, err = tlv.readOctetString(1)
	if err != nil {
		panic(err)
	}
	out.responderRandom, err = tlv.readOctetString(2)
	if err != nil {
		panic(err)
	}
	responderSession, err := tlv.readUInt(3)
	if err != nil {
		panic(err)
	}
	out.responderSession = int(responderSession)
	log.Println(tlv.data.Available())
	log.Println(tlv.data.Bytes())
	log.Println(hex.EncodeToString(tlv.data.Bytes()))
	err = tlv.checkAndSkipBytes([]byte{0x35, 0x4})
	if err != nil {
		panic(err)
	}
	iterations, err := tlv.readUInt(1)
	if err != nil {
		panic(err)
	}
	out.iterations = int(iterations)
	out.salt, err = tlv.readOctetString(2)
	if err != nil {
		panic(err)
	}
	log.Println(hex.EncodeToString(tlv.data.Bytes()))
	out.dump()

	var o AllResp
	o.PBKDFParamResponse = &out

	return o
}

func decodePAKE2ParamResponse(buf *bytes.Buffer) AllResp {
	log.Println("decoding pake2")
	var out PAKE2ParamResponse
	var tlv TLVBufferDec
	tlv.data = buf
	err := tlv.checkAndSkip(0x15)
	if err != nil {
		panic(err)
	}
	out.pb, err = tlv.readOctetString(1)
	if err != nil {
		panic(err)
	}
	out.cb, err = tlv.readOctetString(2)
	if err != nil {
		panic(err)
	}

	var o AllResp
	o.PAKE2ParamResponse = &out

	return o
}

func Pake1ParamRequest(key []byte) []byte {
	var buffer bytes.Buffer
	msg := Message {
		sessionId: 0x0,
		securityFlags: 0,
		messageCounter: 3,
		sourceNodeId: []byte{1,2,3,4,5,6,7,8},
		prot: ProtocolMessage{
			exchangeFlags: 5,
			opcode: SEC_CHAN_OPCODE_PAKE1,
			exchangeId: 0xba3e,
			protocolId: 0x00,
		},
	}
	msg.encode(&buffer)

	var tlv TLVBuffer
	tlv.writeAnonStruct()
	tlv.writeOctetString(0x1, key)
	tlv.writeAnonStructEnd()
	buffer.Write(tlv.data.Bytes())
	return buffer.Bytes()
}

func Pake3ParamRequest(key []byte) []byte {
	var buffer bytes.Buffer
	msg := Message {
		sessionId: 0x0,
		securityFlags: 0,
		messageCounter: 5,
		sourceNodeId: []byte{1,2,3,4,5,6,7,8},
		prot: ProtocolMessage{
			exchangeFlags: 5,
			//exchangeFlags: 7,
			opcode: SEC_CHAN_OPCODE_PAKE3,
			exchangeId: 0xba3e,
			protocolId: 0x00,
		},
	}
	msg.encode(&buffer)

	var tlv TLVBuffer
	tlv.writeAnonStruct()
	tlv.writeOctetString(0x1, key)
	tlv.writeAnonStructEnd()
	buffer.Write(tlv.data.Bytes())
	return buffer.Bytes()
}

func Ack(cnt uint32, counter uint32) []byte {
	var buffer bytes.Buffer
	msg := Message {
		sessionId: 0x0,
		securityFlags: 0,
		messageCounter: cnt,
		sourceNodeId: []byte{1,2,3,4,5,6,7,8},
		prot: ProtocolMessage{
			exchangeFlags: 3,
			//exchangeFlags: 7,
			opcode: SEC_CHAN_OPCODE_ACK,
			exchangeId: 0xba3e,
			protocolId: 0x00,
		},
	}
	msg.encode(&buffer)
	binary.Write(&buffer, binary.LittleEndian, counter)


	return buffer.Bytes()
}

func decode(data []byte) AllResp {
	var msg Message
	buf := bytes.NewBuffer(data)
	msg.decode(buf)
	msg.dump()

	switch msg.prot.protocolId {
	case PROTOCOL_ID_SECURE_CHANNEL:
		switch msg.prot.opcode {
		case SEC_CHAN_OPCODE_PBKDF_RESP:
			resp := decodePBKDFParamResponse(buf)
			resp.messageCounter = msg.messageCounter
			return resp
		case SEC_CHAN_OPCODE_PAKE2:
			resp := decodePAKE2ParamResponse(buf)
			resp.messageCounter = msg.messageCounter
			return resp
		case SEC_CHAN_OPCODE_STATUS_REP:
			resp := decodeStatusReport(buf)
			resp.messageCounter = msg.messageCounter
			return resp
		}
	}
	return AllResp{}
}


func Secured(session uint16, counter uint32) []byte {
	var buffer bytes.Buffer
	msg := Message {
		sessionId: session,
		securityFlags: 0,
		messageCounter: counter,
		sourceNodeId: []byte{1,2,3,4,5,6,7,8},
	}
	msg.encodeBase(&buffer)
	return buffer.Bytes()
}