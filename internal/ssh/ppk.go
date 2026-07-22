package ssh

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/ssh"
)

// ErrNotPPK 表示传入的密钥数据不是 PPK 格式
var ErrNotPPK = errors.New("not a PuTTY PPK key file")

// ppkHeader 存储从 PPK 文件解析出的头部信息和数据块
type ppkHeader struct {
	version    int    // PPK 版本：2 或 3
	keyType    string // 密钥类型，如 ssh-rsa、ssh-ed25519
	encryption string // 加密方式：none / aes256cbc / aes256ctr
	comment    string // 密钥注释
	keyDeriv   string // v3 的 Argon2 密钥派生参数
	publicBlob []byte // 公钥二进制数据
	privateRaw []byte // 私钥原始数据（可能加密）
	macHex     string // MAC/Hash 十六进制字符串
}

// parsePPK 解析 PuTTY PPK 格式的私钥并返回 ssh.Signer。
// 支持 PPK v2（none / aes256cbc）和 PPK v3（argon2id + aes256ctr）。
// 如果数据不是 PPK 格式，返回 ErrNotPPK。
func parsePPK(data []byte, passphrase string) (ssh.Signer, error) {
	header, err := parsePPKText(data)
	if err != nil {
		return nil, err
	}

	// 解密私钥数据
	privateBlob, err := decryptPrivateBlob(header, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypt ppk private blob: %w", err)
	}

	// 验证 MAC
	if err := verifyPPKMAC(header, privateBlob, passphrase); err != nil {
		return nil, fmt.Errorf("ppk mac verification failed: %w", err)
	}

	// 从解密后的数据构造 ssh.Signer
	return constructSigner(header.keyType, header.publicBlob, privateBlob)
}

// parsePPKText 解析 PPK 文件的文本格式，提取头部信息和数据块
func parsePPKText(data []byte) (*ppkHeader, error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return nil, ErrNotPPK
	}

	// 检查首行，判断 PPK 版本
	firstLine := strings.TrimSpace(lines[0])
	var version int
	switch {
	case strings.HasPrefix(firstLine, "PuTTY-User-Key-File-2:"):
		version = 2
	case strings.HasPrefix(firstLine, "PuTTY-User-Key-File-3:"):
		version = 3
	default:
		return nil, ErrNotPPK
	}

	header := &ppkHeader{version: version}

	// 解析首行中的密钥类型
	parts := strings.SplitN(firstLine, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid ppk header line: %s", firstLine)
	}
	header.keyType = strings.TrimSpace(parts[1])

	// 逐行解析
	i := 1
	for i < len(lines) {
		line := strings.TrimRight(lines[i], "\r")
		i++

		if line == "" {
			continue
		}

		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		switch key {
		case "Encryption":
			header.encryption = value
		case "Comment":
			header.comment = value
		case "Key-Derivation":
			header.keyDeriv = value
		case "Public-Lines":
			count, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Public-Lines count: %w", err)
			}
			header.publicBlob, i = readBlobLines(lines, i, count)
		case "Private-Lines":
			count, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Private-Lines count: %w", err)
			}
			header.privateRaw, i = readBlobLines(lines, i, count)
		case "Private-MAC":
			header.macHex = value
		case "Private-Hash":
			header.macHex = value
		}
	}

	if len(header.publicBlob) == 0 || len(header.privateRaw) == 0 {
		return nil, fmt.Errorf("ppk file missing public or private data")
	}

	return header, nil
}

// readBlobLines 从指定位置读取 count 行 base64 数据并解码
func readBlobLines(lines []string, start, count int) ([]byte, int) {
	var sb strings.Builder
	pos := start
	for j := 0; j < count && pos < len(lines); j++ {
		sb.WriteString(strings.TrimRight(lines[pos], "\r"))
		pos++
	}
	decoded, err := base64.StdEncoding.DecodeString(sb.String())
	if err != nil {
		return nil, pos
	}
	return decoded, pos
}

// decryptPrivateBlob 根据 PPK 版本和加密方式解密私钥数据
func decryptPrivateBlob(header *ppkHeader, passphrase string) ([]byte, error) {
	if header.encryption == "none" {
		// 未加密，直接返回原始数据
		return header.privateRaw, nil
	}

	switch header.version {
	case 2:
		return decryptPPKv2(header, passphrase)
	case 3:
		return decryptPPKv3(header, passphrase)
	default:
		return nil, fmt.Errorf("unsupported ppk version: %d", header.version)
	}
}

// decryptPPKv2 使用 AES-256-CBC 解密 PPK v2 私钥数据
// 密钥派生方式：SHA1 前缀计数器模式
func decryptPPKv2(header *ppkHeader, passphrase string) ([]byte, error) {
	if header.encryption != "aes256cbc" {
		return nil, fmt.Errorf("unsupported ppk v2 encryption: %s", header.encryption)
	}

	// 派生 32 字节 AES-256 密钥
	// key1 = SHA1(uint32_be(0) + passphrase)
	// key2 = SHA1(uint32_be(1) + passphrase)
	// cipher_key = key1 + key2（取前 32 字节）
	key1 := sha1Hash(append([]byte{0, 0, 0, 0}, []byte(passphrase)...))
	key2 := sha1Hash(append([]byte{0, 0, 0, 1}, []byte(passphrase)...))
	cipherKey := append(key1, key2...)[:32]

	// CBC 模式的 IV 全为零
	iv := make([]byte, aes.BlockSize)

	block, err := aes.NewCipher(cipherKey)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}

	if len(header.privateRaw)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("encrypted data length %d is not a multiple of block size", len(header.privateRaw))
	}

	decrypted := make([]byte, len(header.privateRaw))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(decrypted, header.privateRaw)

	// 移除 PKCS#7 填充
	return removePKCS7Padding(decrypted, aes.BlockSize)
}

// decryptPPKv3 使用 Argon2 派生密钥 + AES-256-CTR 解密 PPK v3 私钥数据
func decryptPPKv3(header *ppkHeader, passphrase string) ([]byte, error) {
	if header.encryption != "aes256ctr" {
		return nil, fmt.Errorf("unsupported ppk v3 encryption: %s", header.encryption)
	}

	// 解析 Argon2 参数
	// 格式：argon2id@<memory>,<passes>,<parallelism>,<salt_base64>
	cipherKey, iv, _, err := derivePPKv3Keys(header.keyDeriv, passphrase)
	if err != nil {
		return nil, fmt.Errorf("derive v3 keys: %w", err)
	}

	block, err := aes.NewCipher(cipherKey)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}

	decrypted := make([]byte, len(header.privateRaw))
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(decrypted, header.privateRaw)

	// v3 使用 PKCS#7 填充
	return removePKCS7Padding(decrypted, aes.BlockSize)
}

// derivePPKv3Keys 从 Argon2 参数派生加密密钥、IV 和 MAC 密钥
func derivePPKv3Keys(keyDeriv, passphrase string) (cipherKey, iv, macKey []byte, err error) {
	// 解析 Key-Derivation 字段
	// 格式：argon2id@<memory_kib>,<passes>,<parallelism>,<salt_base64>
	if !strings.HasPrefix(keyDeriv, "argon2") {
		return nil, nil, nil, fmt.Errorf("unsupported key derivation: %s", keyDeriv)
	}

	atIdx := strings.Index(keyDeriv, "@")
	if atIdx < 0 {
		return nil, nil, nil, fmt.Errorf("invalid key derivation format")
	}

	algo := keyDeriv[:atIdx]       // argon2id 或 argon2i
	params := keyDeriv[atIdx+1:]  // memory,passes,parallelism,salt

	parts := strings.SplitN(params, ",", 4)
	if len(parts) != 4 {
		return nil, nil, nil, fmt.Errorf("invalid argon2 parameters")
	}

	memory, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse argon2 memory: %w", err)
	}
	passes, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse argon2 passes: %w", err)
	}
	parallelism, err := strconv.ParseUint(parts[2], 10, 8)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse argon2 parallelism: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("decode argon2 salt: %w", err)
	}

	// 派生 80 字节：32(cipher_key) + 16(iv) + 32(mac_key)
	var derived []byte
	switch algo {
	case "argon2id":
		derived = argon2.IDKey(
			[]byte(passphrase), salt,
			uint32(passes), uint32(memory), uint8(parallelism),
			80,
		)
	case "argon2i":
		derived = argon2.Key(
			[]byte(passphrase), salt,
			uint32(passes), uint32(memory), uint8(parallelism),
			80,
		)
	default:
		return nil, nil, nil, fmt.Errorf("unsupported argon2 variant: %s", algo)
	}

	return derived[0:32], derived[32:48], derived[48:80], nil
}

// verifyPPKMAC 验证 PPK 文件的 MAC/Hash
func verifyPPKMAC(header *ppkHeader, privateBlob []byte, passphrase string) error {
	expectedMAC, err := hexDecode(header.macHex)
	if err != nil {
		return fmt.Errorf("decode mac hex: %w", err)
	}

	var macKey []byte
	var computedMAC []byte

	if header.version == 2 {
		// v2: HMAC-SHA1
		// mac_key = SHA1("putty-private-key-file-mac-key" + passphrase)
		macKey = sha1Hash(append([]byte("putty-private-key-file-mac-key"), []byte(passphrase)...))

		var buf bytes.Buffer
		writeSSHStringStr(&buf, header.keyType)
		writeSSHStringStr(&buf, header.encryption)
		writeSSHStringStr(&buf, header.comment)
		writeSSHString(&buf, header.publicBlob)
		writeSSHString(&buf, privateBlob)

		h := hmac.New(sha1.New, macKey)
		h.Write(buf.Bytes())
		computedMAC = h.Sum(nil)
	} else {
		// v3: HMAC-SHA-256
		if header.encryption == "none" {
			// v3 未加密时使用空 MAC 密钥
			macKey = []byte{}
		} else {
			_, _, macKey, err = derivePPKv3Keys(header.keyDeriv, passphrase)
			if err != nil {
				return fmt.Errorf("derive v3 mac key: %w", err)
			}
		}

		var buf bytes.Buffer
		buf.WriteString("ppk3")
		writeSSHStringStr(&buf, header.keyType)
		writeSSHStringStr(&buf, header.encryption)
		writeSSHStringStr(&buf, header.comment)
		writeSSHString(&buf, header.publicBlob)
		writeSSHString(&buf, privateBlob)

		h := hmac.New(sha256.New, macKey)
		h.Write(buf.Bytes())
		computedMAC = h.Sum(nil)
	}

	if !hmac.Equal(expectedMAC, computedMAC) {
		return fmt.Errorf("mac mismatch (wrong passphrase or corrupted key)")
	}

	return nil
}

// constructSigner 从 PPK 公钥和私钥数据构造 ssh.Signer
func constructSigner(keyType string, publicBlob, privateBlob []byte) (ssh.Signer, error) {
	switch keyType {
	case "ssh-rsa":
		return constructRSASigner(publicBlob, privateBlob)
	case "ssh-ed25519":
		return constructEd25519Signer(publicBlob, privateBlob)
	case "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
		return constructECDSASigner(keyType, publicBlob, privateBlob)
	default:
		return nil, fmt.Errorf("unsupported ppk key type: %s", keyType)
	}
}

// constructRSASigner 从 PPK 数据构造 RSA 签名器
// 公钥: mpint(e), mpint(n)
// 私钥: mpint(d), mpint(p), mpint(q), mpint(iqmp)
func constructRSASigner(publicBlob, privateBlob []byte) (ssh.Signer, error) {
	pr := newWireReader(publicBlob)
	if _, err := pr.readString(); err != nil { // 跳过 key type 字符串
		return nil, fmt.Errorf("read rsa key type: %w", err)
	}
	e, err := pr.readMPInt()
	if err != nil {
		return nil, fmt.Errorf("read rsa exponent: %w", err)
	}
	n, err := pr.readMPInt()
	if err != nil {
		return nil, fmt.Errorf("read rsa modulus: %w", err)
	}

	pvr := newWireReader(privateBlob)
	d, err := pvr.readMPInt()
	if err != nil {
		return nil, fmt.Errorf("read rsa private exponent: %w", err)
	}
	p, err := pvr.readMPInt()
	if err != nil {
		return nil, fmt.Errorf("read rsa prime p: %w", err)
	}
	q, err := pvr.readMPInt()
	if err != nil {
		return nil, fmt.Errorf("read rsa prime q: %w", err)
	}

	rsaKey := &rsa.PrivateKey{
		PublicKey: rsa.PublicKey{
			N: n,
			E: int(e.Int64()),
		},
		D:      d,
		Primes: []*big.Int{p, q},
	}
	rsaKey.Precompute()

	return ssh.NewSignerFromKey(rsaKey)
}

// constructEd25519Signer 从 PPK 数据构造 Ed25519 签名器
// 公钥: string(pub 32 bytes)
// 私钥: string(seed 32 bytes)
func constructEd25519Signer(publicBlob, privateBlob []byte) (ssh.Signer, error) {
	pr := newWireReader(publicBlob)
	if _, err := pr.readString(); err != nil { // 跳过 key type 字符串
		return nil, fmt.Errorf("read ed25519 key type: %w", err)
	}
	pub, err := pr.readString()
	if err != nil {
		return nil, fmt.Errorf("read ed25519 public key: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid ed25519 public key size: %d", len(pub))
	}

	pvr := newWireReader(privateBlob)
	seed, err := pvr.readString()
	if err != nil {
		return nil, fmt.Errorf("read ed25519 private seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid ed25519 seed size: %d", len(seed))
	}

	// 从种子派生完整私钥（64 字节 = 种子 + 公钥）
	privKey := ed25519.NewKeyFromSeed(seed)
	return ssh.NewSignerFromKey(privKey)
}

// constructECDSASigner 从 PPK 数据构造 ECDSA 签名器
// 公钥: string(curve_name), string(Q point)
// 私钥: mpint(D)
func constructECDSASigner(keyType string, publicBlob, privateBlob []byte) (ssh.Signer, error) {
	var curve elliptic.Curve
	var curveName string
	switch keyType {
	case "ecdsa-sha2-nistp256":
		curve = elliptic.P256()
		curveName = "nistp256"
	case "ecdsa-sha2-nistp384":
		curve = elliptic.P384()
		curveName = "nistp384"
	case "ecdsa-sha2-nistp521":
		curve = elliptic.P521()
		curveName = "nistp521"
	}

	pr := newWireReader(publicBlob)
	if _, err := pr.readString(); err != nil { // 跳过 key type 字符串
		return nil, fmt.Errorf("read ecdsa key type: %w", err)
	}
	cn, err := pr.readString()
	if err != nil {
		return nil, fmt.Errorf("read ecdsa curve name: %w", err)
	}
	if string(cn) != curveName {
		return nil, fmt.Errorf("curve name mismatch: expected %s, got %s", curveName, string(cn))
	}
	qBytes, err := pr.readString()
	if err != nil {
		return nil, fmt.Errorf("read ecdsa public point: %w", err)
	}
	x, y := elliptic.UnmarshalCompressed(curve, qBytes)
	if x == nil {
		// 尝试非压缩格式
		x, y = elliptic.Unmarshal(curve, qBytes)
		if x == nil {
			return nil, fmt.Errorf("failed to unmarshal ecdsa public point")
		}
	}

	pvr := newWireReader(privateBlob)
	d, err := pvr.readMPInt()
	if err != nil {
		return nil, fmt.Errorf("read ecdsa private scalar: %w", err)
	}

	ecdsaKey := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: curve,
			X:     x,
			Y:     y,
		},
		D: d,
	}

	return ssh.NewSignerFromKey(ecdsaKey)
}

// === SSH wire format 辅助工具 ===

// wireReader 解析 SSH wire 格式的二进制数据
type wireReader struct {
	data []byte
	pos  int
}

func newWireReader(data []byte) *wireReader {
	return &wireReader{data: data}
}

// readString 读取一个长度前缀字符串
func (r *wireReader) readString() ([]byte, error) {
	if r.pos+4 > len(r.data) {
		return nil, fmt.Errorf("unexpected end of data reading string length")
	}
	length := int(binary.BigEndian.Uint32(r.data[r.pos:]))
	r.pos += 4
	if r.pos+length > len(r.data) {
		return nil, fmt.Errorf("string length %d exceeds remaining data", length)
	}
	s := make([]byte, length)
	copy(s, r.data[r.pos:r.pos+length])
	r.pos += length
	return s, nil
}

// readMPInt 读取一个多精度整数
func (r *wireReader) readMPInt() (*big.Int, error) {
	s, err := r.readString()
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(s), nil
}

// writeSSHString 写入一个长度前缀字符串到 buffer
func writeSSHString(buf *bytes.Buffer, s []byte) {
	binary.Write(buf, binary.BigEndian, uint32(len(s)))
	buf.Write(s)
}

// writeSSHStringStr 写入一个字符串到 buffer
func writeSSHStringStr(buf *bytes.Buffer, s string) {
	writeSSHString(buf, []byte(s))
}

// === 辅助函数 ===

// sha1Hash 计算 SHA1 哈希
func sha1Hash(data []byte) []byte {
	h := sha1.Sum(data)
	return h[:]
}

// removePKCS7Padding 移除 PKCS#7 填充
func removePKCS7Padding(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	if len(data)%blockSize != 0 {
		return nil, fmt.Errorf("data length %d not multiple of block size %d", len(data), blockSize)
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize {
		return nil, fmt.Errorf("invalid padding length: %d", padding)
	}
	for i := len(data) - padding; i < len(data); i++ {
		if int(data[i]) != padding {
			return nil, fmt.Errorf("invalid padding bytes")
		}
	}
	return data[:len(data)-padding], nil
}

// hexDecode 解析十六进制字符串
func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd length hex string")
	}
	result := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		high, err := hexDigit(s[i])
		if err != nil {
			return nil, err
		}
		low, err := hexDigit(s[i+1])
		if err != nil {
			return nil, err
		}
		result[i/2] = (high << 4) | low
	}
	return result, nil
}

func hexDigit(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	default:
		return 0, fmt.Errorf("invalid hex digit: %c", c)
	}
}
