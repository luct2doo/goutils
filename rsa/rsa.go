package rsa

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
)

// RSAHelper 是一个 RSA 加密解密辅助结构体。
// 【生命周期说明】：
// 该结构体内部没有保存任何状态（无状态设计），因此它是线程安全的。
// 你可以在程序启动时实例化一次，然后在整个应用程序的生命周期内重复使用它，
// 不需要担心内存泄漏或并发冲突问题。
type RSAHelper struct{}

// NewRSAHelper 创建并返回一个 RSAHelper 实例。
// 【为什么这样设计】：虽然可以直接使用 &RSAHelper{}，但提供一个 New 函数符合 Go 语言的惯用法，
// 并且如果未来需要扩展（例如在初始化时预加载密钥），这里就是最佳的扩展点。
func NewRSAHelper() *RSAHelper {
	return &RSAHelper{}
}

// =============================================================================
// 1. 密钥生成模块
// =============================================================================

// GenerateKeyPair 生成 RSA 公私钥对，并返回 PEM 格式的字符串。
//
// 【函数功能描述】：
// 生成指定比特长度（如 2048 或 4096）的 RSA 密钥对，并将其编码为业界标准的 PEM 格式字符串。
// PEM 格式是以 "-----BEGIN..." 开头的纯文本格式，非常适合保存在配置文件或数据库中。
//
// 【参数说明】：
//   - bits: 密钥长度。强烈建议使用 2048 或 4096。低于 2048 被认为是不安全的。
//
// 【返回值】：
//   - publicKeyPEM: 公钥的 PEM 格式字符串。
//   - privateKeyPEM: 私钥的 PEM 格式字符串。
//   - error: 如果生成过程中发生错误（如随机数生成器故障），则返回错误信息。
//
// 【调用示例】：
//
//	helper := rsahelper.New()
//	pubKey, privKey, err := helper.GenerateKeyPair(2048)
func (h *RSAHelper) GenerateKeyPair(bits int) (publicKeyPEM, privateKeyPEM string, err error) {
	// 1. 使用 crypto/rand 生成安全的随机数来创建私钥。
	// 【为什么使用 rand.Reader】：密码学操作必须使用密码学安全的伪随机数生成器 (CSPRNG)，
	// 普通的 math/rand 是可预测的，绝对不能用于生成密钥。
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return "", "", fmt.Errorf("生成 RSA 私钥失败: %w", err)
	}

	// 2. 将私钥转换为 PKCS#1 格式的 ASN.1 DER 编码字节切片。
	// 【为什么用 PKCS#1】：这是 RSA 密钥最基础、最广泛支持的标准格式。
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)

	// 3. 将 DER 编码的字节切片封装为 PEM 格式。
	// 【为什么用 PEM】：DER 是二进制格式，不便阅读和传输。PEM 是 Base64 编码的文本格式，
	// 带有明确的头部和尾部标识，方便人类阅读和系统存储。
	privateKeyBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	// 将 PEM 块编码为字节切片，再转换为字符串
	privateKeyPEM = string(pem.EncodeToMemory(privateKeyBlock))

	// 4. 从私钥中提取公钥。
	// 【为什么这样获取公钥】：RSA 的公钥是私钥的一部分，直接从生成的私钥结构体中获取即可，无需重新生成。
	publicKey := &privateKey.PublicKey
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", "", fmt.Errorf("序列化公钥失败: %w", err)
	}

	// 5. 将公钥也封装为 PEM 格式。
	// 【注意】：公钥的 Type 通常标记为 "PUBLIC KEY" (PKIX格式)，而不是 "RSA PUBLIC KEY"。
	publicKeyBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	publicKeyPEM = string(pem.EncodeToMemory(publicKeyBlock))

	return publicKeyPEM, privateKeyPEM, nil
}

// =============================================================================
// 2. 加解密模块
// =============================================================================

// EncryptWithPublicKey 使用公钥对明文进行加密。
//
// 【函数功能描述】：
// 接收明文字符串和 PEM 格式的公钥，使用 RSA-OAEP 填充方案进行加密，并返回 Base64 编码的密文。
//
// 【参数说明】：
//   - plaintext: 需要加密的原始明文字符串。
//   - publicKeyPEM: PEM 格式的公钥字符串。
//
// 【返回值】：
//   - ciphertextBase64: Base64 编码的加密后字符串（方便在网络 JSON 中传输）。
//   - error: 解析公钥失败或加密失败时返回错误。
//
// 【调用示例】：
//
//	cipherText, err := helper.EncryptWithPublicKey("hello world", pubKey)
//
// 【⚠️ 新手必读：长度限制警告】：
// RSA 加密对数据长度有严格限制！对于 2048 位密钥 + SHA256 的 OAEP 填充，
// 最大可加密的明文长度约为：2048/8 - 2*32 - 2 = 214 字节。
// 如果需要加密长文本（如长 JSON），请不要直接使用 RSA，而应使用 "RSA + AES 混合加密"：
// 即用 AES 加密长文本，再用 RSA 加密 AES 的密钥。
func (h *RSAHelper) EncryptWithPublicKey(plaintext string, publicKeyPEM string) (ciphertextBase64 string, err error) {
	// 1. 从 PEM 字符串中解析出公钥对象
	publicKey, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return "", err
	}

	// 2. 执行加密操作
	// 【为什么使用 OAEP】：OAEP (Optimal Asymmetric Encryption Padding) 是目前推荐的、
	// 能够抵抗多种密码学攻击（如选择密文攻击）的安全填充方案。比老旧的 PKCS1v15 更安全。
	// 参数说明：rand.Reader(随机源), publicKey(公钥), sha256.New()(哈希函数), nil(标签，通常为nil), []byte(明文)
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, []byte(plaintext), nil)
	if err != nil {
		// 捕获并提示长度超限的常见错误
		if err.Error() == "crypto/rsa: message too long for RSA public key size" {
			return "", errors.New("加密失败：明文太长，超出了 RSA 密钥的长度限制。请考虑使用 AES 混合加密。")
		}
		return "", fmt.Errorf("RSA 加密失败: %w", err)
	}

	// 3. 将二进制密文转换为 Base64 字符串，方便存储和传输
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptWithPrivateKey 使用私钥对密文进行解密。
//
// 【函数功能描述】：
// 接收 Base64 编码的密文和 PEM 格式的私钥，使用 RSA-OAEP 填充方案进行解密，返回原始明文字符串。
//
// 【参数说明】：
//   - ciphertextBase64: Base64 编码的加密字符串。
//   - privateKeyPEM: PEM 格式的私钥字符串。
//
// 【返回值】：
//   - plaintext: 解密后的原始明文字符串。
//   - error: 解析私钥失败、Base64解码失败或解密失败时返回错误。
//
// 【调用示例】：
//
//	plainText, err := helper.DecryptWithPrivateKey(cipherText, privKey)
func (h *RSAHelper) DecryptWithPrivateKey(ciphertextBase64 string, privateKeyPEM string) (plaintext string, err error) {
	// 1. 从 PEM 字符串中解析出私钥对象
	privateKey, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}

	// 2. 将 Base64 字符串解码回二进制密文
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return "", fmt.Errorf("Base64 解码失败: %w", err)
	}

	// 3. 执行解密操作
	// 【注意】：这里的哈希函数 (sha256.New()) 和标签 (nil) 必须与加密时完全一致，否则解密会失败。
	plaintextBytes, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("RSA 解密失败 (可能是密钥不匹配或密文被篡改): %w", err)
	}

	return string(plaintextBytes), nil
}

// EncryptWithPublicKeyPKCS1v15 使用公钥按 PKCS#1 v1.5 填充加密，并自动分块。
//
// 【什么时候用它，而不是 EncryptWithPublicKey】：
// EncryptWithPublicKey 走的是 OAEP 填充，密文与 PKCS#1 v1.5 互不兼容 ——
// 对方用 PHP `openssl_public_encrypt()`（默认 OPENSSL_PKCS1_PADDING）、
// Java `Cipher.getInstance("RSA/ECB/PKCS1Padding")`、Node `crypto.publicEncrypt` 默认值
// 加密过来的数据，**只能**用本函数解；反过来也一样。
//
// 【为什么要分块】：
// 单个 RSA 块能加密的明文上限是 `密钥字节数 - 11`（2048 位 = 245 字节，1024 位 = 117 字节）。
// 超长明文必须切块、逐块加密、再把密文按顺序拼接后统一 Base64。
// 这与 PHP 里 `foreach (str_split($data, 117) as $chunk)` 的做法完全一致，
// 块大小由密钥自动推导，不需要调用方写死 117。
//
// 【参数说明】：
//   - plaintext: 明文（可为空串，返回空串）
//   - publicKeyPEM: PEM 格式公钥（PKIX "PUBLIC KEY" 或 PKCS#1 "RSA PUBLIC KEY"）
//
// 【返回值】：
//   - 所有分块密文拼接后的 Base64 字符串
//   - error: 解析公钥失败或加密失败
func (h *RSAHelper) EncryptWithPublicKeyPKCS1v15(plaintext string, publicKeyPEM string) (string, error) {
	publicKey, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return "", err
	}

	if plaintext == "" {
		return "", nil
	}

	// 单块明文上限：k - 11（PKCS#1 v1.5 需要 0x00 0x02 + 至少 8 字节随机填充 + 0x00）
	blockSize := publicKey.Size() - 11
	if blockSize <= 0 {
		return "", errors.New("密钥长度过短，无法使用 PKCS#1 v1.5 填充")
	}

	plain := []byte(plaintext)
	blocks := (len(plain) + blockSize - 1) / blockSize
	ciphertext := make([]byte, 0, blocks*publicKey.Size())

	for offset := 0; offset < len(plain); offset += blockSize {
		end := offset + blockSize
		if end > len(plain) {
			end = len(plain)
		}
		encrypted, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, plain[offset:end])
		if err != nil {
			return "", fmt.Errorf("RSA(PKCS#1 v1.5) 分块加密失败 (第 %d 块): %w", offset/blockSize+1, err)
		}
		ciphertext = append(ciphertext, encrypted...)
	}

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptWithPrivateKeyPKCS1v15 使用私钥按 PKCS#1 v1.5 填充解密，并自动分块。
//
// 【说明】：与 EncryptWithPublicKeyPKCS1v15 配对使用，用于解开 PHP `openssl_private_decrypt()`
// 或任何 PKCS#1 v1.5 实现产生的密文。密文长度必须能被密钥字节数整除，否则说明数据被截断或不是分块格式。
//
// 【⚠️ 安全提示】：
// PKCS#1 v1.5 解密存在 padding oracle 风险（攻击者能通过区分「填充错误」与「内容错误」逐步解出明文）。
// 若本函数用于对外的解密接口，**务必**把成功与失败统一成同一个错误返回，不要向外暴露具体原因。
func (h *RSAHelper) DecryptWithPrivateKeyPKCS1v15(ciphertextBase64 string, privateKeyPEM string) (string, error) {
	privateKey, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}

	if ciphertextBase64 == "" {
		return "", nil
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return "", fmt.Errorf("Base64 解码失败: %w", err)
	}

	blockSize := privateKey.Size()
	if len(ciphertext)%blockSize != 0 {
		return "", fmt.Errorf("密文长度 %d 不是密钥块长 %d 的整数倍（数据被截断或分块格式不符）",
			len(ciphertext), blockSize)
	}

	plain := make([]byte, 0, len(ciphertext))
	for offset := 0; offset < len(ciphertext); offset += blockSize {
		decrypted, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey,
			ciphertext[offset:offset+blockSize])
		if err != nil {
			return "", fmt.Errorf("RSA(PKCS#1 v1.5) 分块解密失败 (第 %d 块): %w",
				offset/blockSize+1, err)
		}
		plain = append(plain, decrypted...)
	}

	return string(plain), nil
}

// =============================================================================
// 3. 签名与验签模块 (用于身份认证和数据完整性校验)
// =============================================================================

// SignWithPrivateKey 使用私钥对数据进行数字签名。
//
// 【函数功能描述】：
// 对传入的数据计算 SHA256 哈希，然后使用私钥对该哈希值进行签名。
// 常用于证明“这段数据确实是我发出的，且中途未被篡改”。
//
// 【参数说明】：
//   - data: 需要签名的原始数据字符串。
//   - privateKeyPEM: PEM 格式的私钥字符串。
//
// 【返回值】：
//   - signatureBase64: Base64 编码的签名字符串。
//   - error: 解析私钥或签名失败时返回错误。
func (h *RSAHelper) SignWithPrivateKey(data string, privateKeyPEM string) (signatureBase64 string, err error) {
	privateKey, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}

	// 1. 计算数据的 SHA256 哈希值
	hashed := sha256.Sum256([]byte(data))

	// 2. 使用私钥对哈希值进行签名
	// 【为什么用 PKCS1v15】：虽然 PSS 是更新的签名方案，但 PKCS1v15 在目前的 API 交互中兼容性最好，
	// 且对于 SHA256 来说依然是安全的。
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("RSA 签名失败: %w", err)
	}

	return base64.StdEncoding.EncodeToString(signature), nil
}

// VerifyWithPublicKey 使用公钥验证数字签名。
//
// 【函数功能描述】：
// 使用公钥验证提供的签名是否是由对应的私钥对指定数据生成的。
//
// 【参数说明】：
//   - data: 原始数据字符串（必须与签名时的数据完全一致）。
//   - signatureBase64: Base64 编码的签名字符串。
//   - publicKeyPEM: PEM 格式的公钥字符串。
//
// 【返回值】：
//   - isValid: 布尔值，true 表示验证通过，false 表示验证失败。
//   - error: 解析公钥、Base64解码失败或验签过程中发生的系统错误。
func (h *RSAHelper) VerifyWithPublicKey(data string, signatureBase64 string, publicKeyPEM string) (isValid bool, err error) {
	publicKey, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return false, err
	}

	signature, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return false, fmt.Errorf("Base64 解码签名失败: %w", err)
	}

	hashed := sha256.Sum256([]byte(data))

	// 3. 验证签名
	// 【返回值说明】：rsa.VerifyPKCS1v15 如果验证成功，会返回 nil；如果失败，会返回具体的错误。
	err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signature)
	if err != nil {
		// 验证失败不一定是系统错误，可能是数据被篡改或密钥不匹配，这里返回 false 和 nil error 更符合业务逻辑
		return false, nil
	}

	return true, nil
}

// =============================================================================
// 4. 内部辅助函数 (不对外暴露，保持 API 简洁)
// =============================================================================

// parsePrivateKey 内部辅助函数：从 PEM 字符串解析出 *rsa.PrivateKey
func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("无效的 PEM 格式：未找到 PEM 块")
	}

	// 尝试解析 PKCS#1 格式 (传统 RSA 私钥)
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return privateKey, nil
	}

	// 如果失败，尝试解析 PKCS#8 格式 (更通用的私钥格式，某些工具生成的可能是这种)
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析私钥失败 (既不是 PKCS#1 也不是 PKCS#8): %w", err)
	}

	// 类型断言，确保解析出来的是 RSA 私钥
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("解析出的私钥不是 RSA 类型")
	}

	return rsaKey, nil
}

// parsePublicKey 内部辅助函数：从 PEM 字符串解析出 *rsa.PublicKey
func parsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("无效的 PEM 格式：未找到 PEM 块")
	}

	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析公钥失败: %w", err)
	}

	pubKey, ok := pubInterface.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("解析出的公钥不是 RSA 类型")
	}

	return pubKey, nil
}
