package gomat

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"fmt"
	"log"
	randm "math/rand"
	"net"

	"github.com/finnigja/gomat/mattertlv"
)

// Spake2pExchange establishes secure session using PASE (Passcode-Authenticated Session Establishment).
// This uses SPAKE2+ protocol
func Spake2pExchange(pin int, udp *udpChannel) (SecureChannel, error) {
	exchange := uint16(randm.Intn(0xffff))
	secure_channel := SecureChannel{
		Udp:     udp,
		session: 0,
		Counter: uint32(randm.Intn(0xffffffff)),
	}

	pbkdf_request := pBKDFParamRequest(exchange)
	secure_channel.Send(pbkdf_request)

	pbkdf_responseS, err := secure_channel.Receive()
	if err != nil {
		return SecureChannel{}, fmt.Errorf("pbkdf response not received: %s", err.Error())
	}
	if pbkdf_responseS.ProtocolHeader.Opcode != SEC_CHAN_OPCODE_PBKDF_RESP {
		return SecureChannel{}, fmt.Errorf("SEC_CHAN_OPCODE_PBKDF_RESP not received")
	}
	pbkdf_response_salt := pbkdf_responseS.Tlv.GetOctetStringRec([]int{4, 2})
	pbkdf_response_iterations, err := pbkdf_responseS.Tlv.GetIntRec([]int{4, 1})
	if err != nil {
		return SecureChannel{}, fmt.Errorf("can't get pbkdf_response_iterations")
	}
	pbkdf_response_session, err := pbkdf_responseS.Tlv.GetIntRec([]int{3})
	if err != nil {
		return SecureChannel{}, fmt.Errorf("can't get pbkdf_response_session")
	}

	sctx := NewSpaceCtx()
	sctx.Gen_w(pin, pbkdf_response_salt, int(pbkdf_response_iterations))
	sctx.Gen_random_X()
	sctx.Calc_X()

	pake1 := pake1ParamRequest(exchange, sctx.X.As_bytes())
	secure_channel.Send(pake1)

	pake2s, err := secure_channel.Receive()
	if err != nil {
		return SecureChannel{}, fmt.Errorf("pake2 not received: %s", err.Error())
	}
	if pake2s.ProtocolHeader.Opcode != SEC_CHAN_OPCODE_PAKE2 {
		return SecureChannel{}, fmt.Errorf("SEC_CHAN_OPCODE_PAKE2 not received")
	}
	//pake2s.tlv.Dump(1)
	pake2_pb := pake2s.Tlv.GetOctetStringRec([]int{1})

	sctx.Y.from_bytes(pake2_pb)
	sctx.calc_ZV()
	ttseed := []byte("CHIP PAKE V1 Commissioning")
	ttseed = append(ttseed, pbkdf_request[6:]...) // 6 is size of proto header
	ttseed = append(ttseed, pbkdf_responseS.Payload...)
	err = sctx.calc_hash(ttseed)
	if err != nil {
		return SecureChannel{}, err
	}

	pake3 := pake3ParamRequest(exchange, sctx.cA)
	secure_channel.Send(pake3)

	status_report, err := secure_channel.Receive()
	if err != nil {
		return SecureChannel{}, err
	}
	if status_report.StatusReport.ProtocolCode != 0 {
		return SecureChannel{}, fmt.Errorf("pake3 is not success code: %d", status_report.StatusReport.ProtocolCode)
	}

	secure_channel = SecureChannel{
		Udp:         udp,
		decrypt_key: sctx.decrypt_key,
		encrypt_key: sctx.encrypt_key,
		remote_node: []byte{0, 0, 0, 0, 0, 0, 0, 0},
		local_node:  []byte{0, 0, 0, 0, 0, 0, 0, 0},
		session:     int(pbkdf_response_session),
	}

	return secure_channel, nil
}

// SigmaExhange establishes secure session using CASE (Certificate Authenticated Session Establishment)
func SigmaExchange(fabric *Fabric, controller_id uint64, device_id uint64, secure_channel SecureChannel) (SecureChannel, error) {

	controller_privkey, _ := ecdh.P256().GenerateKey(rand.Reader)
	sigma_context := sigmaContext{
		session_privkey: controller_privkey,
		exchange:        uint16(randm.Intn(0xffff)),
	}
	sigma_context.genSigma1(fabric, device_id)
	sigma1 := genSigma1Req2(sigma_context.sigma1payload, sigma_context.exchange)
	secure_channel.Send(sigma1)

	var err error
	sigma_context.sigma2dec, err = secure_channel.Receive()
	if err != nil {
		return SecureChannel{}, err
	}
	if (sigma_context.sigma2dec.ProtocolHeader.ProtocolId == ProtocolIdSecureChannel) &&
		(sigma_context.sigma2dec.ProtocolHeader.Opcode == SEC_CHAN_OPCODE_STATUS_REP) {
		return SecureChannel{}, fmt.Errorf("sigma2 not received. status: %x %x", sigma_context.sigma2dec.StatusReport.GeneralCode,
			sigma_context.sigma2dec.StatusReport.ProtocolCode)
	}
	if sigma_context.sigma2dec.ProtocolHeader.Opcode != 0x31 {
		return SecureChannel{}, fmt.Errorf("sigma2 not received")
	}

	sigma_context.controller_key, err = fabric.CertificateManager.GetPrivkey(controller_id)
	if err != nil {
		return SecureChannel{}, err
	}
	controller_cert, err := fabric.CertificateManager.GetCertificate(controller_id)
	if err != nil {
		return SecureChannel{}, err
	}
	sigma_context.controller_matter_certificate = SerializeCertificateIntoMatter(fabric, controller_cert)

	to_send, err := sigma_context.sigma3(fabric)
	if err != nil {
		return SecureChannel{}, err
	}
	secure_channel.Send(to_send)

	sigma_result, err := secure_channel.Receive()
	if err != nil {
		return SecureChannel{}, err
	}
	if sigma_result.ProtocolHeader.Opcode != SEC_CHAN_OPCODE_STATUS_REP {
		return SecureChannel{}, fmt.Errorf("unexpected message (opcode:0x%x)", sigma_result.ProtocolHeader.Opcode)
	}
	if !sigma_result.StatusReport.IsOk() {
		return SecureChannel{}, fmt.Errorf("sigma result is not ok %d %d %d",
			sigma_result.StatusReport.GeneralCode,
			sigma_result.StatusReport.ProtocolId, sigma_result.StatusReport.ProtocolCode)
	}

	secure_channel.decrypt_key = sigma_context.r2ikey
	secure_channel.encrypt_key = sigma_context.i2rkey
	secure_channel.remote_node = id_to_bytes(device_id)
	secure_channel.local_node = id_to_bytes(controller_id)
	secure_channel.session = sigma_context.session
	return secure_channel, nil
}

// Commission performs commissioning procedure on device with device_ip ip address
//   - fabric is fabric object with approriate certificate authority
//   - pin is passcode used for device pairing
//   - controller_id is identifier of node whioch will be owner/admin of this device
//   - device_id_id is identifier of "new" device
func Commission(fabric *Fabric, device_ip net.IP, pin int, controller_id, device_id uint64) error {

	channel, err := startUdpChannel(device_ip, 5540, 55555)
	if err != nil {
		return err
	}
	secure_channel := SecureChannel{
		Udp: channel,
	}
	defer secure_channel.Close()

	secure_channel, err = Spake2pExchange(pin, channel)
	if err != nil {
		return err
	}

	// send csr request
	var tlvb mattertlv.TLVBuffer
	tlvb.WriteOctetString(0, CreateRandomBytes(32))
	to_send := EncodeIMInvokeRequest(0, 0x3e, 4, tlvb.Bytes(), false, uint16(randm.Intn(0xffff)))
	secure_channel.Send(to_send)

	csr_resp, err := secure_channel.Receive()
	if err != nil {
		return err
	}

	nocsr := csr_resp.Tlv.GetOctetStringRec([]int{1, 0, 0, 1, 0})
	if len(nocsr) == 0 {
		return fmt.Errorf("nocsr not received")
	}
	tlv2 := mattertlv.Decode(nocsr)
	csr := tlv2.GetOctetStringRec([]int{1})
	csrp, err := x509.ParseCertificateRequest(csr)
	if err != nil {
		return err
	}

	//AddTrustedRootCertificate
	var tlv4 mattertlv.TLVBuffer
	tlv4.WriteOctetString(0, SerializeCertificateIntoMatter(fabric, fabric.CertificateManager.GetCaCertificate()))
	to_send = EncodeIMInvokeRequest(0, 0x3e, 0xb, tlv4.Bytes(), false, uint16(randm.Intn(0xffff)))
	secure_channel.Send(to_send)

	resp, err := secure_channel.Receive()
	if err != nil {
		return err
	}
	resp_status := ParseImInvokeResponse(&resp.Tlv)
	if resp_status != 0 {
		return fmt.Errorf("unexpected status to AddTrustedRootCertificate %d", resp_status)
	}

	//noc_x509 := sign_cert(csrp, 2, "user")
	noc_x509, err := fabric.CertificateManager.SignCertificate(csrp.PublicKey.(*ecdsa.PublicKey), device_id)
	if err != nil {
		return err
	}
	noc_matter := SerializeCertificateIntoMatter(fabric, noc_x509)
	//AddNOC
	var tlv5 mattertlv.TLVBuffer
	tlv5.WriteOctetString(0, noc_matter)
	tlv5.WriteOctetString(2, fabric.ipk) //ipk
	tlv5.WriteUInt64(3, controller_id)   // admin subject !
	tlv5.WriteUInt16(4, 101)             // admin vendorid ??
	to_send = EncodeIMInvokeRequest(0, 0x3e, 0x6, tlv5.Bytes(), false, uint16(randm.Intn(0xffff)))

	secure_channel.Send(to_send)

	resp, err = secure_channel.Receive()
	if err != nil {
		return err
	}
	resp_status_add_noc, err := resp.Tlv.GetIntRec([]int{1, 0, 0, 1, 0})
	if err != nil {
		return fmt.Errorf("error during AddNOC %s", err.Error())
	}
	if resp_status_add_noc != 0 {
		return fmt.Errorf("unexpected status to AddNOC %d", resp_status_add_noc)
	}

	secure_channel.decrypt_key = []byte{}
	secure_channel.encrypt_key = []byte{}
	secure_channel.session = 0

	secure_channel, err = SigmaExchange(fabric, controller_id, device_id, secure_channel)
	if err != nil {
		return err
	}

	//commissioning complete
	to_send = EncodeIMInvokeRequest(0, 0x30, 4, []byte{}, false, uint16(randm.Intn(0xffff)))
	secure_channel.Send(to_send)

	respx, err := secure_channel.Receive()
	if err != nil {
		return err
	}
	commissioning_result, err := respx.Tlv.GetIntRec([]int{1, 0, 0, 1, 0})
	if err != nil {
		return err
	}
	if commissioning_result == 0 {
		log.Printf("commissioning OK\n")
	} else {
		log.Printf("commissioning error: %d\n", commissioning_result)
	}
	return nil
}

func ConnectDevice(device_ip net.IP, port int, fabric *Fabric, device_id, admin_id uint64) (SecureChannel, error) {
	var secure_channel SecureChannel
	var err error
	if secure_channel, err = StartSecureChannel(device_ip, port, 55555); err != nil {
		return SecureChannel{}, err
	}
	if secure_channel, err = SigmaExchange(fabric, admin_id, device_id, secure_channel); err != nil {
		return SecureChannel{}, err
	}
	return secure_channel, nil
}
