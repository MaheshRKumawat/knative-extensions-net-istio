package resources

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/google/go-cmp/cmp"
	"knative.dev/net-istio/pkg/reconciler/ingress/config"
)

func TestMakeMirrorSecret(t *testing.T) {
	mockSecret := func(cert, key string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-secret-name",
				Namespace: "some-namespace",
			},
			Data: map[string][]byte{
				corev1.TLSCertKey:       []byte(cert),
				corev1.TLSPrivateKeyKey: []byte(key),
			},
		}
	}

	cases := []struct {
		name            string
		certificateHash string
		inputCert       string
		inputKey        string
		expectCert      string
		expectKey       string
	}{
		{
			name:            "mirror secret name is provided certificate hash and it is in the Istio namespace still having the same data content",
			certificateHash: "deadbeef42",
			inputCert: `-----BEGIN CERTIFICATE-----
MIICaDCCAe6gAwIBAgIURp4drpZU3OZJnNtZ64MOAdUdYmYwCgYIKoZIzj0EAwMw
ajELMAkGA1UEBhMCRkIxFDASBgNVBAgMC0Zvb2JhcnNoaXJlMRIwEAYDVQQKDAlG
b29iYXJJbmMxEDAOBgNVBAcMB0Zvb3Rvd24xEjAQBgNVBAMMCWxvY2FsaG9zdDEL
MAkGA1UECwwCN0cwIBcNMjQwNTEwMTIzMzM2WhgPMjEyNDA1MTAxMjMzMzZaMGox
CzAJBgNVBAYTAkZCMRQwEgYDVQQIDAtGb29iYXJzaGlyZTESMBAGA1UECgwJRm9v
YmFySW5jMRAwDgYDVQQHDAdGb290b3duMRIwEAYDVQQDDAlsb2NhbGhvc3QxCzAJ
BgNVBAsMAjdHMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEu8y/1M5jQq7ydkJr2k5y
vQwXwF/Pf+ilNMwc0vdPLbew1OZ6TAgPA0kHiPSKj1mLOsTvtiiNV2/37xNfqW6+
f98T5jduPpJRQddBizijhjl7eMrqQqeALK2xAKlhULRJo1MwUTAdBgNVHQ4EFgQU
McixolMNtDEqtC/OZjk9MzqZqJkwHwYDVR0jBBgwFoAUMcixolMNtDEqtC/OZjk9
MzqZqJkwDwYDVR0TAQH/BAUwAwEB/zAKBggqhkjOPQQDAwNoADBlAjAo9+gsBXkR
u4zkrorgPWCd5Ys7jlwl8r+6lZdzsuq9qBs6rXR5Gfe7rWwXbJuyCqQCMQCs5k9D
fej8FW0LmqTNltAvORQ6Iagrlj2DolI3UiHFXoQ6r+dkwJmUK8nKvFlU8FQ=
-----END CERTIFICATE-----
`,
			inputKey: `-----BEGIN EC PARAMETERS-----
BgUrgQQAIg==
-----END EC PARAMETERS-----
-----BEGIN EC PRIVATE KEY-----
MIGkAgEBBDBYViANw6YT3JTdeXs1TwWH29Lij8TTdYhvyaNphj0PwDUHMwcoEtvJ
bcOVSWblplOgBwYFK4EEACKhZANiAAS7zL/UzmNCrvJ2QmvaTnK9DBfAX89/6KU0
zBzS908tt7DU5npMCA8DSQeI9IqPWYs6xO+2KI1Xb/fvE1+pbr5/3xPmN24+klFB
10GLOKOGOXt4yupCp4AsrbEAqWFQtEk=
-----END EC PRIVATE KEY-----
`,
			expectCert: `-----BEGIN CERTIFICATE-----
MIICaDCCAe6gAwIBAgIURp4drpZU3OZJnNtZ64MOAdUdYmYwCgYIKoZIzj0EAwMw
ajELMAkGA1UEBhMCRkIxFDASBgNVBAgMC0Zvb2JhcnNoaXJlMRIwEAYDVQQKDAlG
b29iYXJJbmMxEDAOBgNVBAcMB0Zvb3Rvd24xEjAQBgNVBAMMCWxvY2FsaG9zdDEL
MAkGA1UECwwCN0cwIBcNMjQwNTEwMTIzMzM2WhgPMjEyNDA1MTAxMjMzMzZaMGox
CzAJBgNVBAYTAkZCMRQwEgYDVQQIDAtGb29iYXJzaGlyZTESMBAGA1UECgwJRm9v
YmFySW5jMRAwDgYDVQQHDAdGb290b3duMRIwEAYDVQQDDAlsb2NhbGhvc3QxCzAJ
BgNVBAsMAjdHMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEu8y/1M5jQq7ydkJr2k5y
vQwXwF/Pf+ilNMwc0vdPLbew1OZ6TAgPA0kHiPSKj1mLOsTvtiiNV2/37xNfqW6+
f98T5jduPpJRQddBizijhjl7eMrqQqeALK2xAKlhULRJo1MwUTAdBgNVHQ4EFgQU
McixolMNtDEqtC/OZjk9MzqZqJkwHwYDVR0jBBgwFoAUMcixolMNtDEqtC/OZjk9
MzqZqJkwDwYDVR0TAQH/BAUwAwEB/zAKBggqhkjOPQQDAwNoADBlAjAo9+gsBXkR
u4zkrorgPWCd5Ys7jlwl8r+6lZdzsuq9qBs6rXR5Gfe7rWwXbJuyCqQCMQCs5k9D
fej8FW0LmqTNltAvORQ6Iagrlj2DolI3UiHFXoQ6r+dkwJmUK8nKvFlU8FQ=
-----END CERTIFICATE-----
`,
			expectKey: `-----BEGIN EC PARAMETERS-----
BgUrgQQAIg==
-----END EC PARAMETERS-----
-----BEGIN EC PRIVATE KEY-----
MIGkAgEBBDBYViANw6YT3JTdeXs1TwWH29Lij8TTdYhvyaNphj0PwDUHMwcoEtvJ
bcOVSWblplOgBwYFK4EEACKhZANiAAS7zL/UzmNCrvJ2QmvaTnK9DBfAX89/6KU0
zBzS908tt7DU5npMCA8DSQeI9IqPWYs6xO+2KI1Xb/fvE1+pbr5/3xPmN24+klFB
10GLOKOGOXt4yupCp4AsrbEAqWFQtEk=
-----END EC PRIVATE KEY-----
`,
		},
		{
			name:            "mirror secret name is provided certificate hash and it is in the Istio namespace still having cert and key with additional lines, should properly format cert and key",
			certificateHash: "deadbeef42",
			inputCert: `-----BEGIN CERTIFICATE-----

MIICaDCCAe6gAwIBAgIURp4drpZU3OZJnNtZ64MOAdUdYmYwCgYIKoZIzj0EAwMw

ajELMAkGA1UEBhMCRkIxFDASBgNVBAgMC0Zvb2JhcnNoaXJlMRIwEAYDVQQKDAlG

b29iYXJJbmMxEDAOBgNVBAcMB0Zvb3Rvd24xEjAQBgNVBAMMCWxvY2FsaG9zdDEL

MAkGA1UECwwCN0cwIBcNMjQwNTEwMTIzMzM2WhgPMjEyNDA1MTAxMjMzMzZaMGox

CzAJBgNVBAYTAkZCMRQwEgYDVQQIDAtGb29iYXJzaGlyZTESMBAGA1UECgwJRm9v

YmFySW5jMRAwDgYDVQQHDAdGb290b3duMRIwEAYDVQQDDAlsb2NhbGhvc3QxCzAJ

BgNVBAsMAjdHMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEu8y/1M5jQq7ydkJr2k5y

vQwXwF/Pf+ilNMwc0vdPLbew1OZ6TAgPA0kHiPSKj1mLOsTvtiiNV2/37xNfqW6+

f98T5jduPpJRQddBizijhjl7eMrqQqeALK2xAKlhULRJo1MwUTAdBgNVHQ4EFgQU

McixolMNtDEqtC/OZjk9MzqZqJkwHwYDVR0jBBgwFoAUMcixolMNtDEqtC/OZjk9

MzqZqJkwDwYDVR0TAQH/BAUwAwEB/zAKBggqhkjOPQQDAwNoADBlAjAo9+gsBXkR

u4zkrorgPWCd5Ys7jlwl8r+6lZdzsuq9qBs6rXR5Gfe7rWwXbJuyCqQCMQCs5k9D

fej8FW0LmqTNltAvORQ6Iagrlj2DolI3UiHFXoQ6r+dkwJmUK8nKvFlU8FQ=

-----END CERTIFICATE-----
`,
			inputKey: `-----BEGIN EC PARAMETERS-----

BgUrgQQAIg==

-----END EC PARAMETERS-----

-----BEGIN EC PRIVATE KEY-----

MIGkAgEBBDBYViANw6YT3JTdeXs1TwWH29Lij8TTdYhvyaNphj0PwDUHMwcoEtvJ

bcOVSWblplOgBwYFK4EEACKhZANiAAS7zL/UzmNCrvJ2QmvaTnK9DBfAX89/6KU0

zBzS908tt7DU5npMCA8DSQeI9IqPWYs6xO+2KI1Xb/fvE1+pbr5/3xPmN24+klFB

10GLOKOGOXt4yupCp4AsrbEAqWFQtEk=

-----END EC PRIVATE KEY-----
`,
			expectCert: `-----BEGIN CERTIFICATE-----
MIICaDCCAe6gAwIBAgIURp4drpZU3OZJnNtZ64MOAdUdYmYwCgYIKoZIzj0EAwMw
ajELMAkGA1UEBhMCRkIxFDASBgNVBAgMC0Zvb2JhcnNoaXJlMRIwEAYDVQQKDAlG
b29iYXJJbmMxEDAOBgNVBAcMB0Zvb3Rvd24xEjAQBgNVBAMMCWxvY2FsaG9zdDEL
MAkGA1UECwwCN0cwIBcNMjQwNTEwMTIzMzM2WhgPMjEyNDA1MTAxMjMzMzZaMGox
CzAJBgNVBAYTAkZCMRQwEgYDVQQIDAtGb29iYXJzaGlyZTESMBAGA1UECgwJRm9v
YmFySW5jMRAwDgYDVQQHDAdGb290b3duMRIwEAYDVQQDDAlsb2NhbGhvc3QxCzAJ
BgNVBAsMAjdHMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEu8y/1M5jQq7ydkJr2k5y
vQwXwF/Pf+ilNMwc0vdPLbew1OZ6TAgPA0kHiPSKj1mLOsTvtiiNV2/37xNfqW6+
f98T5jduPpJRQddBizijhjl7eMrqQqeALK2xAKlhULRJo1MwUTAdBgNVHQ4EFgQU
McixolMNtDEqtC/OZjk9MzqZqJkwHwYDVR0jBBgwFoAUMcixolMNtDEqtC/OZjk9
MzqZqJkwDwYDVR0TAQH/BAUwAwEB/zAKBggqhkjOPQQDAwNoADBlAjAo9+gsBXkR
u4zkrorgPWCd5Ys7jlwl8r+6lZdzsuq9qBs6rXR5Gfe7rWwXbJuyCqQCMQCs5k9D
fej8FW0LmqTNltAvORQ6Iagrlj2DolI3UiHFXoQ6r+dkwJmUK8nKvFlU8FQ=
-----END CERTIFICATE-----
`,
			expectKey: `-----BEGIN EC PARAMETERS-----
BgUrgQQAIg==
-----END EC PARAMETERS-----
-----BEGIN EC PRIVATE KEY-----
MIGkAgEBBDBYViANw6YT3JTdeXs1TwWH29Lij8TTdYhvyaNphj0PwDUHMwcoEtvJ
bcOVSWblplOgBwYFK4EEACKhZANiAAS7zL/UzmNCrvJ2QmvaTnK9DBfAX89/6KU0
zBzS908tt7DU5npMCA8DSQeI9IqPWYs6xO+2KI1Xb/fvE1+pbr5/3xPmN24+klFB
10GLOKOGOXt4yupCp4AsrbEAqWFQtEk=
-----END EC PRIVATE KEY-----
`,
		},
		{
			name:            "mirror secret name is provided certificate hash and it is in the Istio namespace still having multiple certs and keys with additional lines, should properly format certs and keys",
			certificateHash: "deadbeef42",
			inputCert: `-----BEGIN CERTIFICATE-----

MIICaDCCAe6gAwIBAgIURp4drpZU3OZJnNtZ64MOAdUdYmYwCgYIKoZIzj0EAwMw

ajELMAkGA1UEBhMCRkIxFDASBgNVBAgMC0Zvb2JhcnNoaXJlMRIwEAYDVQQKDAlG

b29iYXJJbmMxEDAOBgNVBAcMB0Zvb3Rvd24xEjAQBgNVBAMMCWxvY2FsaG9zdDEL

MAkGA1UECwwCN0cwIBcNMjQwNTEwMTIzMzM2WhgPMjEyNDA1MTAxMjMzMzZaMGox

CzAJBgNVBAYTAkZCMRQwEgYDVQQIDAtGb29iYXJzaGlyZTESMBAGA1UECgwJRm9v

YmFySW5jMRAwDgYDVQQHDAdGb290b3duMRIwEAYDVQQDDAlsb2NhbGhvc3QxCzAJ

BgNVBAsMAjdHMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEu8y/1M5jQq7ydkJr2k5y

vQwXwF/Pf+ilNMwc0vdPLbew1OZ6TAgPA0kHiPSKj1mLOsTvtiiNV2/37xNfqW6+

f98T5jduPpJRQddBizijhjl7eMrqQqeALK2xAKlhULRJo1MwUTAdBgNVHQ4EFgQU

McixolMNtDEqtC/OZjk9MzqZqJkwHwYDVR0jBBgwFoAUMcixolMNtDEqtC/OZjk9

MzqZqJkwDwYDVR0TAQH/BAUwAwEB/zAKBggqhkjOPQQDAwNoADBlAjAo9+gsBXkR

u4zkrorgPWCd5Ys7jlwl8r+6lZdzsuq9qBs6rXR5Gfe7rWwXbJuyCqQCMQCs5k9D

fej8FW0LmqTNltAvORQ6Iagrlj2DolI3UiHFXoQ6r+dkwJmUK8nKvFlU8FQ=

-----END CERTIFICATE-----

-----BEGIN CERTIFICATE-----

MIIFlzCCA3+gAwIBAgIUfJF+t0/Bn4tJsmIIPZnyjf1sfgIwDQYJKoZIhvcNAQEL

BQAwWzELMAkGA1UEBhMCQVUxCjAIBgNVBAgMAXMxCjAIBgNVBAcMAWExCjAIBgNV

BAoMAWExCjAIBgNVBAsMAWExCjAIBgNVBAMMAWExEDAOBgkqhkiG9w0BCQEWAWEw

HhcNMjQwNTAzMTEzNzM1WhcNMjUwNTAzMTEzNzM1WjBbMQswCQYDVQQGEwJBVTEK

MAgGA1UECAwBczEKMAgGA1UEBwwBYTEKMAgGA1UECgwBYTEKMAgGA1UECwwBYTEK

MAgGA1UEAwwBYTEQMA4GCSqGSIb3DQEJARYBYTCCAiIwDQYJKoZIhvcNAQEBBQAD

ggIPADCCAgoCggIBAL/cQN4EpT5TOxQmop146wQ8XCRdV4Y2FwQpnfyihSMIv+ec

ROKqxSiiVGKcioUfcNuJmeias4ZZ2CoR+plAQH5fqnt/YW+QQKet36hGDn2Y95UO

vjHfUb5BgnwBT1Ld0a6BTxLigStfbxfMmLaXYfEvEiPSg6tMSbiHX7Okw/IfjF33

mALECD9IConBSZXWJfNzySCRJXpJ176IUFOIaHrUkoVUWLIbykDSAJQlTTiwpqhO

7bGwmaYk5IwTWov8S+CcOjQQMohoK6+2djO1mrOVxeYXI0HSd0d4qy1GuSldO3iB

8TOA4jdxUrYPS5JGnQJN94gcChgf9wW63Nzhkg8y6JL36pUBFBWfJzl1OqT/1Aq4

2rS5LHpdHsjglfkIBkIWNseK07WxkzY7mebD6wopB3zRKVvQGZxm69TtFrQ205as

Fvh5lpYVtdPNIvvQJwZi9n4XS4uoLh+isPzvX8binyAejIVNB0PauPUUihfeEI39

vgpRJtsIW1CmJAvHmGMyRM2Bjgjl0WESjFIZQYOCYilw1svRqyhiBaomvI8i/a9Q

DZY4LY6ZsAjnX6M3N0vW2O7q6dahJpR2FvoFWS5kSlW1Tt1fJuZs1weCKIFI3SX1

VblY9grRCRwooLiDH+lRC7W5zhlX+fkpy1Cb3UtJxDN9SEJC7dYMOCTJQVF1AgMB

AAGjUzBRMB0GA1UdDgQWBBSE4lrYtiSeVzL6HpkuHMPw4CechzAfBgNVHSMEGDAW

gBSE4lrYtiSeVzL6HpkuHMPw4CechzAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3

DQEBCwUAA4ICAQCtTEhtRWS9ywmG3Cy5dMlai5mpjWQp0zh5um+YThV/Ifzh3y47

z411oSUQ3rCKm2LDIv2KLHlus9sD+UIg53fJrbdKS/ACWAdg1uHMMnOLIoG+0KxR

CBtTfZ9p4Djw7cC1kTgRL8WcK9kyC/UpG9Aixn62iU+Bj9BExKsQW7kzH7ULhukW

4prgHva3eZY3KY3hfFBjGz1GDeMRxO01ULbOZnUtKqX+NyFi5e+0oah6isZt+E3V

rbWTdxH/PS8lZ0s+69mrcW9Umsu9tdgN4MQaKsxKwk0G6xdh+Qfz/Od9xF6v0cq6

46KouLeTdHbLZvIfuMMF+PkF2kxI+LC4WkxSnK0Y485uPqwjiGFk2UzSHKPFD1LK

JkJjrSceMu7MqQh98pKUjrv353kqg2i3rmEcc/6r6UKdn5kyRihKBmvVAjkWD1Mb

k+Dd75OSYm6HQFPjsiv5D1mqsexoehoCzNTMJGp0ofHWakgCSP9kpYOGg4Gh2S09

xBOAnwXjLWCUfAcDtMGJPSGGxPkQ4y2kJ8zIehxWfiV2zUoIypGEvWmT4sdn3uwO

n36+wfVYHoyNxAuZo35jAZWAF91DE/Sn9Br0ls+SiJPJXt7G73GI/C5uOx28Z4m3

wqZFw/pS80aiQYwqum5Cldo/aLbz4VN8JCO/oPrjHeLjwBhayFzO2egtFw==

-----END CERTIFICATE-----
`,
			inputKey: `-----BEGIN EC PARAMETERS-----

BgUrgQQAIg==

-----END EC PARAMETERS-----

-----BEGIN EC PRIVATE KEY-----

MIGkAgEBBDBYViANw6YT3JTdeXs1TwWH29Lij8TTdYhvyaNphj0PwDUHMwcoEtvJ

bcOVSWblplOgBwYFK4EEACKhZANiAAS7zL/UzmNCrvJ2QmvaTnK9DBfAX89/6KU0

zBzS908tt7DU5npMCA8DSQeI9IqPWYs6xO+2KI1Xb/fvE1+pbr5/3xPmN24+klFB

10GLOKOGOXt4yupCp4AsrbEAqWFQtEk=

-----END EC PRIVATE KEY-----

-----BEGIN EC PRIVATE KEY-----

MIhsfdwy3746egd2x7t3e7t72etxqw7g3qxw73e7367tr7g32t7t3DUHMwcoEtvJ

bcOVSWbwy3746egd2x7t3e7t72etxqw7g3qxfdeCrvJ2QmvaTnK9DBfAX89/6KU0

zBzS908tt7DU5npMCAwy3746egd2x7t3e7t72etxqw7g3qxw73e/3xPmN24+klFB

10GLOKOGOXt4yupCp4AsrbEAqWFQtEk=

-----END EC PRIVATE KEY-----
`,
			expectCert: `-----BEGIN CERTIFICATE-----
MIICaDCCAe6gAwIBAgIURp4drpZU3OZJnNtZ64MOAdUdYmYwCgYIKoZIzj0EAwMw
ajELMAkGA1UEBhMCRkIxFDASBgNVBAgMC0Zvb2JhcnNoaXJlMRIwEAYDVQQKDAlG
b29iYXJJbmMxEDAOBgNVBAcMB0Zvb3Rvd24xEjAQBgNVBAMMCWxvY2FsaG9zdDEL
MAkGA1UECwwCN0cwIBcNMjQwNTEwMTIzMzM2WhgPMjEyNDA1MTAxMjMzMzZaMGox
CzAJBgNVBAYTAkZCMRQwEgYDVQQIDAtGb29iYXJzaGlyZTESMBAGA1UECgwJRm9v
YmFySW5jMRAwDgYDVQQHDAdGb290b3duMRIwEAYDVQQDDAlsb2NhbGhvc3QxCzAJ
BgNVBAsMAjdHMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEu8y/1M5jQq7ydkJr2k5y
vQwXwF/Pf+ilNMwc0vdPLbew1OZ6TAgPA0kHiPSKj1mLOsTvtiiNV2/37xNfqW6+
f98T5jduPpJRQddBizijhjl7eMrqQqeALK2xAKlhULRJo1MwUTAdBgNVHQ4EFgQU
McixolMNtDEqtC/OZjk9MzqZqJkwHwYDVR0jBBgwFoAUMcixolMNtDEqtC/OZjk9
MzqZqJkwDwYDVR0TAQH/BAUwAwEB/zAKBggqhkjOPQQDAwNoADBlAjAo9+gsBXkR
u4zkrorgPWCd5Ys7jlwl8r+6lZdzsuq9qBs6rXR5Gfe7rWwXbJuyCqQCMQCs5k9D
fej8FW0LmqTNltAvORQ6Iagrlj2DolI3UiHFXoQ6r+dkwJmUK8nKvFlU8FQ=
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIFlzCCA3+gAwIBAgIUfJF+t0/Bn4tJsmIIPZnyjf1sfgIwDQYJKoZIhvcNAQEL
BQAwWzELMAkGA1UEBhMCQVUxCjAIBgNVBAgMAXMxCjAIBgNVBAcMAWExCjAIBgNV
BAoMAWExCjAIBgNVBAsMAWExCjAIBgNVBAMMAWExEDAOBgkqhkiG9w0BCQEWAWEw
HhcNMjQwNTAzMTEzNzM1WhcNMjUwNTAzMTEzNzM1WjBbMQswCQYDVQQGEwJBVTEK
MAgGA1UECAwBczEKMAgGA1UEBwwBYTEKMAgGA1UECgwBYTEKMAgGA1UECwwBYTEK
MAgGA1UEAwwBYTEQMA4GCSqGSIb3DQEJARYBYTCCAiIwDQYJKoZIhvcNAQEBBQAD
ggIPADCCAgoCggIBAL/cQN4EpT5TOxQmop146wQ8XCRdV4Y2FwQpnfyihSMIv+ec
ROKqxSiiVGKcioUfcNuJmeias4ZZ2CoR+plAQH5fqnt/YW+QQKet36hGDn2Y95UO
vjHfUb5BgnwBT1Ld0a6BTxLigStfbxfMmLaXYfEvEiPSg6tMSbiHX7Okw/IfjF33
mALECD9IConBSZXWJfNzySCRJXpJ176IUFOIaHrUkoVUWLIbykDSAJQlTTiwpqhO
7bGwmaYk5IwTWov8S+CcOjQQMohoK6+2djO1mrOVxeYXI0HSd0d4qy1GuSldO3iB
8TOA4jdxUrYPS5JGnQJN94gcChgf9wW63Nzhkg8y6JL36pUBFBWfJzl1OqT/1Aq4
2rS5LHpdHsjglfkIBkIWNseK07WxkzY7mebD6wopB3zRKVvQGZxm69TtFrQ205as
Fvh5lpYVtdPNIvvQJwZi9n4XS4uoLh+isPzvX8binyAejIVNB0PauPUUihfeEI39
vgpRJtsIW1CmJAvHmGMyRM2Bjgjl0WESjFIZQYOCYilw1svRqyhiBaomvI8i/a9Q
DZY4LY6ZsAjnX6M3N0vW2O7q6dahJpR2FvoFWS5kSlW1Tt1fJuZs1weCKIFI3SX1
VblY9grRCRwooLiDH+lRC7W5zhlX+fkpy1Cb3UtJxDN9SEJC7dYMOCTJQVF1AgMB
AAGjUzBRMB0GA1UdDgQWBBSE4lrYtiSeVzL6HpkuHMPw4CechzAfBgNVHSMEGDAW
gBSE4lrYtiSeVzL6HpkuHMPw4CechzAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3
DQEBCwUAA4ICAQCtTEhtRWS9ywmG3Cy5dMlai5mpjWQp0zh5um+YThV/Ifzh3y47
z411oSUQ3rCKm2LDIv2KLHlus9sD+UIg53fJrbdKS/ACWAdg1uHMMnOLIoG+0KxR
CBtTfZ9p4Djw7cC1kTgRL8WcK9kyC/UpG9Aixn62iU+Bj9BExKsQW7kzH7ULhukW
4prgHva3eZY3KY3hfFBjGz1GDeMRxO01ULbOZnUtKqX+NyFi5e+0oah6isZt+E3V
rbWTdxH/PS8lZ0s+69mrcW9Umsu9tdgN4MQaKsxKwk0G6xdh+Qfz/Od9xF6v0cq6
46KouLeTdHbLZvIfuMMF+PkF2kxI+LC4WkxSnK0Y485uPqwjiGFk2UzSHKPFD1LK
JkJjrSceMu7MqQh98pKUjrv353kqg2i3rmEcc/6r6UKdn5kyRihKBmvVAjkWD1Mb
k+Dd75OSYm6HQFPjsiv5D1mqsexoehoCzNTMJGp0ofHWakgCSP9kpYOGg4Gh2S09
xBOAnwXjLWCUfAcDtMGJPSGGxPkQ4y2kJ8zIehxWfiV2zUoIypGEvWmT4sdn3uwO
n36+wfVYHoyNxAuZo35jAZWAF91DE/Sn9Br0ls+SiJPJXt7G73GI/C5uOx28Z4m3
wqZFw/pS80aiQYwqum5Cldo/aLbz4VN8JCO/oPrjHeLjwBhayFzO2egtFw==
-----END CERTIFICATE-----
`,
			expectKey: `-----BEGIN EC PARAMETERS-----
BgUrgQQAIg==
-----END EC PARAMETERS-----
-----BEGIN EC PRIVATE KEY-----
MIGkAgEBBDBYViANw6YT3JTdeXs1TwWH29Lij8TTdYhvyaNphj0PwDUHMwcoEtvJ
bcOVSWblplOgBwYFK4EEACKhZANiAAS7zL/UzmNCrvJ2QmvaTnK9DBfAX89/6KU0
zBzS908tt7DU5npMCA8DSQeI9IqPWYs6xO+2KI1Xb/fvE1+pbr5/3xPmN24+klFB
10GLOKOGOXt4yupCp4AsrbEAqWFQtEk=
-----END EC PRIVATE KEY-----
-----BEGIN EC PRIVATE KEY-----
MIhsfdwy3746egd2x7t3e7t72etxqw7g3qxw73e7367tr7g32t7t3DUHMwcoEtvJ
bcOVSWbwy3746egd2x7t3e7t72etxqw7g3qxfdeCrvJ2QmvaTnK9DBfAX89/6KU0
zBzS908tt7DU5npMCAwy3746egd2x7t3e7t72etxqw7g3qxw73e/3xPmN24+klFB
10GLOKOGOXt4yupCp4AsrbEAqWFQtEk=
-----END EC PRIVATE KEY-----
`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mirror := MakeMirrorSecret(mockSecret(c.inputCert, c.inputKey), c.certificateHash)

			if diff := cmp.Diff(c.certificateHash, mirror.Name); diff != "" {
				t.Error("Unexpected change in secret name (-want, +got):", diff)
			}

			if diff := cmp.Diff(config.IstioNamespace, mirror.Namespace); diff != "" {
				t.Error("Unexpected change in secret namespace (-want, +got):", diff)
			}

			if _, ok := mirror.Labels["codeengine.cloud.ibm.com/domain-mapping-secret"]; !ok {
				t.Error("Unexpected missing label 'codeengine.cloud.ibm.com/domain-mapping-secret' in mirror secret")
			}

			if diff := cmp.Diff(c.expectCert, string(mirror.Data[corev1.TLSCertKey])); diff != "" {
				t.Error("Unexpected TLS cert in secret (-want, +got):", diff)
			}

			if diff := cmp.Diff(c.expectKey, string(mirror.Data[corev1.TLSPrivateKeyKey])); diff != "" {
				t.Error("Unexpected TLS key in secret (-want, +got):", diff)
			}
		})
	}
}
