package packets

import (
	"fmt"

	ppcrypto "xcthings.com/hjyz/crypto"
)

// Encrypt 加密数据.
func Encrypt(encType uint8, key, iv []byte, payload []byte) (out []byte, err error) {
	if len(key) != 32 || len(iv) != 32 {
		err = fmt.Errorf("key, iv length not match")
		return
	}

	switch encType {
	case AESNONE:
		out = payload
	// CBC
	case AES128CBC:
		out, err = ppcrypto.AesCBCEncrypt(payload, key[:16], iv)
		return
	case AES192CBC:
		out, err = ppcrypto.AesCBCEncrypt(payload, key[:24], iv)
		return
	case AES256CBC:
		out, err = ppcrypto.AesCBCEncrypt(payload, key, iv)
		return
	// CFB
	case AES128CFB:
		out, err = ppcrypto.AesCFBEncrypt(payload, key[:16], iv)
		return
	case AES192CFB:
		out, err = ppcrypto.AesCFBEncrypt(payload, key[:24], iv)
		return
	case AES256CFB:
		out, err = ppcrypto.AesCFBEncrypt(payload, key, iv)
		return
	//
	default:
		err = fmt.Errorf("not support EncType: %d", encType)
	}
	return
}

// Decrypt 解密数据.
func Decrypt(encType uint8, key, iv []byte, payload []byte) (out []byte, err error) {
	if len(key) != 32 || len(iv) != 32 {
		err = fmt.Errorf("key, iv length not match")
		return
	}

	switch encType {
	case AESNONE:
		out = payload
	// CBC
	case AES128CBC:
		out, err = ppcrypto.AesCBCDecrypt(payload, key[:16], iv)
		return
	case AES192CBC:
		out, err = ppcrypto.AesCBCDecrypt(payload, key[:24], iv)
		return
	case AES256CBC:
		out, err = ppcrypto.AesCBCDecrypt(payload, key, iv)
		return
	// CFB
	case AES128CFB:
		out, err = ppcrypto.AesCFBDecrypt(payload, key[:16], iv)
		return
	case AES192CFB:
		out, err = ppcrypto.AesCFBDecrypt(payload, key[:24], iv)
		return
	case AES256CFB:
		out, err = ppcrypto.AesCFBDecrypt(payload, key, iv)
		return
	//
	default:
		err = fmt.Errorf("not support EncType: %d", encType)
	}
	return
}
