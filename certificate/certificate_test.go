// Code on this file adapted from github.com/sideshow/apns2

/*
 * Copyright (c) 2016 Adam Jones
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of
 * this software and associated documentation files (the "Software"), to deal in
 * the Software without restriction, including without limitation the rights to
 * use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
 * the Software, and to permit persons to whom the Software is furnished to do so,
 * subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
 * FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
 * COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
 * IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
 * CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package certificate

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"crypto/tls"
	"io/ioutil"
)

var _ = Describe("Certificate", func() {
	Describe("[Unit]", func() {
		Describe("PKCS#12", func() {
			It("Should load valid certificate from P12File", func() {
				cer, err := FromP12File("../tls/_fixtures/certificate-valid.p12", "")
				Expect(err).NotTo(HaveOccurred())
				// Expect(cer.Leaf).NotTo(BeNil())
				// Expect(cer.Leaf.VerifyHostname("APNS/2 Development IOS Push Services: com.sideshow.Apns2")).To(BeNil())
				Expect(cer).To(Not(Equal(tls.Certificate{})))
			})

			It("Should load valid certificate from P12Bytes", func() {
				bytes, _ := ioutil.ReadFile("../tls/_fixtures/certificate-valid.p12")
				cer, err := FromP12Bytes(bytes, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(cer).To(Not(Equal(tls.Certificate{})))
			})

			It("Should load encrypted certificate from P12File", func() {
				cer, err := FromP12File("../tls/_fixtures/certificate-valid-encrypted.p12", "password")
				Expect(err).NotTo(HaveOccurred())
				Expect(cer).To(Not(Equal(tls.Certificate{})))
			})

			It("Should fail if there is no P12File", func() {
				cer, err := FromP12File("", "")
				Expect(err.Error()).To(Equal("open : no such file or directory"))
				Expect(cer.Certificate).To(BeNil())
			})

			It("Should fail if bad password is provided", func() {
				cer, err := FromP12File("../tls/_fixtures/certificate-valid-encrypted.p12", "")
				Expect(err.Error()).To(Equal("pkcs12: decryption password incorrect"))
				Expect(cer.Certificate).To(BeNil())
			})
		})

		Describe("PEM", func() {
			It("Should load valid certificate from PemFile", func() {
				cer, err := FromPemFile("../tls/_fixtures/certificate-valid.pem", "")
				Expect(err).NotTo(HaveOccurred())
				Expect(cer).To(Not(Equal(tls.Certificate{})))
			})

			It("Should load valid certificate from PemBytes", func() {
				bytes, _ := ioutil.ReadFile("../tls/_fixtures/certificate-valid.pem")
				cer, err := FromPemBytes(bytes, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(cer).To(Not(Equal(tls.Certificate{})))
			})

			It("Should load encrypted certificate from PemFile", func() {
				cer, err := FromPemFile("../tls/_fixtures/certificate-valid-encrypted.pem", "password")
				Expect(err).NotTo(HaveOccurred())
				Expect(cer).To(Not(Equal(tls.Certificate{})))
			})

			It("Should fail if there is no PemFile", func() {
				cer, err := FromPemFile("", "")
				Expect(err.Error()).To(Equal("open : no such file or directory"))
				Expect(cer.Certificate).To(BeNil())
			})

			It("Should fail if bad password is provided", func() {
				cer, err := FromPemFile("../tls/_fixtures/certificate-valid-encrypted.pem", "badpassword")
				Expect(err).To(Equal(ErrFailedToDecryptKey))
				Expect(cer.Certificate).To(BeNil())
			})

			It("Should fail if file with bad key is provided", func() {
				cer, err := FromPemFile("../tls/_fixtures/certificate-bad-key.pem", "")
				Expect(err).To(Equal(ErrFailedToParsePKCS1PrivateKey))
				Expect(cer.Certificate).To(BeNil())
			})

			It("Should fail if file with no key is provided", func() {
				cer, err := FromPemFile("../tls/_fixtures/certificate-no-key.pem", "")
				Expect(err).To(Equal(ErrNoPrivateKey))
				Expect(cer.Certificate).To(BeNil())
			})

			It("Should fail if file with no certificate is provided", func() {
				cer, err := FromPemFile("../tls/_fixtures/certificate-no-certificate.pem", "")
				Expect(err).To(Equal(ErrNoCertificate))
				Expect(cer.Certificate).To(BeNil())
			})
		})
	})
})
