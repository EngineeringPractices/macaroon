package macaroon_test

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"gopkg.in/macaroon.v2-unstable"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type macaroonSuite struct{}

var _ = gc.Suite(&macaroonSuite{})

func never(string) error {
	return fmt.Errorf("condition is never true")
}

func (*macaroonSuite) TestNoCaveats(c *gc.C) {
	rootKey := []byte("secret")
	m := MustNew(rootKey, []byte("some id"), "a location", macaroon.LatestVersion)
	c.Assert(m.Location(), gc.Equals, "a location")
	c.Assert(m.Id(), gc.DeepEquals, []byte("some id"))

	err := m.Verify(rootKey, never, nil)
	c.Assert(err, gc.IsNil)
}

func (*macaroonSuite) TestFirstPartyCaveat(c *gc.C) {
	rootKey := []byte("secret")
	m := MustNew(rootKey, []byte("some id"), "a location", macaroon.LatestVersion)

	caveats := map[string]bool{
		"a caveat":       true,
		"another caveat": true,
	}
	tested := make(map[string]bool)

	for cav := range caveats {
		m.AddFirstPartyCaveat(cav)
	}
	expectErr := fmt.Errorf("condition not met")
	check := func(cav string) error {
		tested[cav] = true
		if caveats[cav] {
			return nil
		}
		return expectErr
	}
	err := m.Verify(rootKey, check, nil)
	c.Assert(err, gc.IsNil)

	c.Assert(tested, gc.DeepEquals, caveats)

	m.AddFirstPartyCaveat("not met")
	err = m.Verify(rootKey, check, nil)
	c.Assert(err, gc.Equals, expectErr)

	c.Assert(tested["not met"], gc.Equals, true)
}

func (*macaroonSuite) TestThirdPartyCaveat(c *gc.C) {
	rootKey := []byte("secret")
	m := MustNew(rootKey, []byte("some id"), "a location", macaroon.LatestVersion)

	dischargeRootKey := []byte("shared root key")
	thirdPartyCaveatId := []byte("3rd party caveat")
	err := m.AddThirdPartyCaveat(dischargeRootKey, thirdPartyCaveatId, "remote.com")
	c.Assert(err, gc.IsNil)

	dm := MustNew(dischargeRootKey, thirdPartyCaveatId, "remote location", macaroon.LatestVersion)
	dm.Bind(m.Signature())
	err = m.Verify(rootKey, never, []*macaroon.Macaroon{dm})
	c.Assert(err, gc.IsNil)
}

func (*macaroonSuite) TestThirdPartyCaveatBadRandom(c *gc.C) {
	rootKey := []byte("secret")
	m := MustNew(rootKey, []byte("some id"), "a location", macaroon.LatestVersion)
	dischargeRootKey := []byte("shared root key")
	thirdPartyCaveatId := []byte("3rd party caveat")

	err := macaroon.AddThirdPartyCaveatWithRand(m, dischargeRootKey, thirdPartyCaveatId, "remote.com", &macaroon.ErrorReader{})
	c.Assert(err, gc.ErrorMatches, "cannot generate random bytes: fail")
}

type conditionTest struct {
	conditions map[string]bool
	expectErr  string
}

var verifyTests = []struct {
	about      string
	macaroons  []macaroonSpec
	conditions []conditionTest
}{{
	about: "single third party caveat without discharge",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful": true,
		},
		expectErr: fmt.Sprintf(`cannot find discharge macaroon for caveat %x`, "bob-is-great"),
	}},
}, {
	about: "single third party caveat with discharge",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful": true,
		},
	}, {
		conditions: map[string]bool{
			"wonderful": false,
		},
		expectErr: `condition "wonderful" not met`,
	}},
}, {
	about: "single third party caveat with discharge with mismatching root key",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key-wrong",
		id:       "bob-is-great",
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful": true,
		},
		expectErr: `signature mismatch after caveat verification`,
	}},
}, {
	about: "single third party caveat with two discharges",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
		caveats: []caveat{{
			condition: "splendid",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
		caveats: []caveat{{
			condition: "top of the world",
		}},
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful": true,
		},
		expectErr: `condition "splendid" not met`,
	}, {
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         true,
			"top of the world": true,
		},
		expectErr: `discharge macaroon "bob-is-great" was not used`,
	}, {
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         false,
			"top of the world": true,
		},
		expectErr: `condition "splendid" not met`,
	}, {
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         true,
			"top of the world": false,
		},
		expectErr: `discharge macaroon "bob-is-great" was not used`,
	}},
}, {
	about: "one discharge used for two macaroons",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "somewhere else",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}, {
			condition: "bob-is-great",
			location:  "charlie",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "somewhere else",
		caveats: []caveat{{
			condition: "bob-is-great",
			location:  "charlie",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
	}},
	conditions: []conditionTest{{
		expectErr: `discharge macaroon "bob-is-great" was used more than once`,
	}},
}, {
	about: "recursive third party caveat",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
		caveats: []caveat{{
			condition: "bob-is-great",
			location:  "charlie",
			rootKey:   "bob-caveat-root-key",
		}},
	}},
	conditions: []conditionTest{{
		expectErr: `discharge macaroon "bob-is-great" was used more than once`,
	}},
}, {
	about: "two third party caveats",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}, {
			condition: "charlie-is-great",
			location:  "charlie",
			rootKey:   "charlie-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
		caveats: []caveat{{
			condition: "splendid",
		}},
	}, {
		location: "charlie",
		rootKey:  "charlie-caveat-root-key",
		id:       "charlie-is-great",
		caveats: []caveat{{
			condition: "top of the world",
		}},
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         true,
			"top of the world": true,
		},
	}, {
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         false,
			"top of the world": true,
		},
		expectErr: `condition "splendid" not met`,
	}, {
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         true,
			"top of the world": false,
		},
		expectErr: `condition "top of the world" not met`,
	}},
}, {
	about: "third party caveat with undischarged third party caveat",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
		caveats: []caveat{{
			condition: "splendid",
		}, {
			condition: "barbara-is-great",
			location:  "barbara",
			rootKey:   "barbara-caveat-root-key",
		}},
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful": true,
			"splendid":  true,
		},
		expectErr: fmt.Sprintf(`cannot find discharge macaroon for caveat %x`, "barbara-is-great"),
	}},
}, {
	about:     "recursive third party caveats",
	macaroons: recursiveThirdPartyCaveatMacaroons,
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful":   true,
			"splendid":    true,
			"high-fiving": true,
			"spiffing":    true,
		},
	}, {
		conditions: map[string]bool{
			"wonderful":   true,
			"splendid":    true,
			"high-fiving": false,
			"spiffing":    true,
		},
		expectErr: `condition "high-fiving" not met`,
	}},
}, {
	about: "unused discharge",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
	}, {
		rootKey: "other-key",
		id:      "unused",
	}},
	conditions: []conditionTest{{
		expectErr: `discharge macaroon "unused" was not used`,
	}},
}}

var recursiveThirdPartyCaveatMacaroons = []macaroonSpec{{
	rootKey: "root-key",
	id:      "root-id",
	caveats: []caveat{{
		condition: "wonderful",
	}, {
		condition: "bob-is-great",
		location:  "bob",
		rootKey:   "bob-caveat-root-key",
	}, {
		condition: "charlie-is-great",
		location:  "charlie",
		rootKey:   "charlie-caveat-root-key",
	}},
}, {
	location: "bob",
	rootKey:  "bob-caveat-root-key",
	id:       "bob-is-great",
	caveats: []caveat{{
		condition: "splendid",
	}, {
		condition: "barbara-is-great",
		location:  "barbara",
		rootKey:   "barbara-caveat-root-key",
	}},
}, {
	location: "charlie",
	rootKey:  "charlie-caveat-root-key",
	id:       "charlie-is-great",
	caveats: []caveat{{
		condition: "splendid",
	}, {
		condition: "celine-is-great",
		location:  "celine",
		rootKey:   "celine-caveat-root-key",
	}},
}, {
	location: "barbara",
	rootKey:  "barbara-caveat-root-key",
	id:       "barbara-is-great",
	caveats: []caveat{{
		condition: "spiffing",
	}, {
		condition: "ben-is-great",
		location:  "ben",
		rootKey:   "ben-caveat-root-key",
	}},
}, {
	location: "ben",
	rootKey:  "ben-caveat-root-key",
	id:       "ben-is-great",
}, {
	location: "celine",
	rootKey:  "celine-caveat-root-key",
	id:       "celine-is-great",
	caveats: []caveat{{
		condition: "high-fiving",
	}},
}}

func (*macaroonSuite) TestVerify(c *gc.C) {
	for i, test := range verifyTests {
		c.Logf("test %d: %s", i, test.about)
		rootKey, primary, discharges := makeMacaroons(test.macaroons)
		for _, cond := range test.conditions {
			c.Logf("conditions %#v", cond.conditions)
			check := func(cav string) error {
				if cond.conditions[cav] {
					return nil
				}
				return fmt.Errorf("condition %q not met", cav)
			}
			err := primary.Verify(
				rootKey,
				check,
				discharges,
			)
			if cond.expectErr != "" {
				c.Assert(err, gc.ErrorMatches, cond.expectErr)
			} else {
				c.Assert(err, gc.IsNil)
			}

			// Cloned macaroon should have same verify result.
			cloneErr := primary.Clone().Verify(rootKey, check, discharges)
			c.Assert(cloneErr, gc.DeepEquals, err)
		}
	}
}

// TODO(rog) move the following JSON-marshal tests into marshal_test.go.

// jsonTestVersions holds the various possible ways of marshaling a macaroon
// to JSON.
var jsonTestVersions = []macaroon.Version{
	macaroon.V1,
	macaroon.V2,
}

func (s *macaroonSuite) TestMarshalJSON(c *gc.C) {
	for i, vers := range jsonTestVersions {
		c.Logf("test %d: %v", i, vers)
		s.testMarshalJSONWithVersion(c, vers)
	}
}

func (*macaroonSuite) testMarshalJSONWithVersion(c *gc.C, vers macaroon.Version) {
	rootKey := []byte("secret")
	m0 := MustNew(rootKey, []byte("some id"), "a location", vers)
	m0.AddFirstPartyCaveat("account = 3735928559")
	m0JSON, err := json.Marshal(m0)
	c.Assert(err, gc.IsNil)
	var m1 macaroon.Macaroon
	err = json.Unmarshal(m0JSON, &m1)
	c.Assert(err, gc.IsNil)
	c.Assert(m0.Location(), gc.Equals, m1.Location())
	c.Assert(string(m0.Id()), gc.Equals, string(m1.Id()))
	c.Assert(
		hex.EncodeToString(m0.Signature()),
		gc.Equals,
		hex.EncodeToString(m1.Signature()))
	c.Assert(m1.Version(), gc.Equals, vers)
}

var jsonRoundTripTests = []struct {
	about string
	// data holds the marshaled data. All the data values hold
	// different encodings of the same macaroon - the same as produced
	// from the second example in libmacaroons
	// example README with the following libmacaroons code:
	//
	// secret = 'this is a different super-secret key; never use the same secret twice'
	// public = 'we used our other secret key'
	// location = 'http://mybank/'
	// M = macaroons.create(location, secret, public)
	// M = M.add_first_party_caveat('account = 3735928559')
	// caveat_key = '4; guaranteed random by a fair toss of the dice'
	// predicate = 'user = Alice'
	// identifier = 'this was how we remind auth of key/pred'
	// M = M.add_third_party_caveat('http://auth.mybank/', caveat_key, identifier)
	// m.serialize_json()
	data                 string
	expectExactRoundTrip bool
	expectVers           macaroon.Version
}{{
	about:                "exact JSON as produced by libmacaroons",
	data:                 `{"caveats":[{"cid":"account = 3735928559"},{"cid":"this was how we remind auth of key\/pred","vid":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA027FAuBYhtHwJ58FX6UlVNFtFsGxQHS7uD_w_dedwv4Jjw7UorCREw5rXbRqIKhr","cl":"http:\/\/auth.mybank\/"}],"location":"http:\/\/mybank\/","identifier":"we used our other secret key","signature":"d27db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c"}`,
	expectVers:           macaroon.V1,
	expectExactRoundTrip: true,
}, {
	about:      "V2 object with hex binary values",
	data:       `{"c":[{"i":"account = 3735928559"},{"i":"this was how we remind auth of key/pred","vH":"000000000000000000000000000000000000000000000000d36ec502e05886d1f0279f055fa52554d16d16c1b14074bbb83ff0fdd79dc2fe098f0ed4a2b091130e6b5db46a20a86b","l":"http://auth.mybank/"}],"l":"http://mybank/","iH":"77652075736564206f7572206f7468657220736563726574206b6579","sH":"d27db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c"}`,
	expectVers: macaroon.V2,
}, {
	about:      "V2 object with std base-64 binary values",
	data:       `{"c":[{"i64":"YWNjb3VudCA9IDM3MzU5Mjg1NTk="},{"i64":"dGhpcyB3YXMgaG93IHdlIHJlbWluZCBhdXRoIG9mIGtleS9wcmVk","v64":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA027FAuBYhtHwJ58FX6UlVNFtFsGxQHS7uD/w/dedwv4Jjw7UorCREw5rXbRqIKhr","l":"http://auth.mybank/"}],"l":"http://mybank/","i64":"d2UgdXNlZCBvdXIgb3RoZXIgc2VjcmV0IGtleQ==","s64":"0n2y/R8idg5MPa6BN+LY/B32wHQcGK7UuXJWv3jR9Vw="}`,
	expectVers: macaroon.V2,
}, {
	about:      "V2 object with URL base-64 binary values",
	data:       `{"c":[{"i64":"YWNjb3VudCA9IDM3MzU5Mjg1NTk"},{"i64":"dGhpcyB3YXMgaG93IHdlIHJlbWluZCBhdXRoIG9mIGtleS9wcmVk","v64":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA027FAuBYhtHwJ58FX6UlVNFtFsGxQHS7uD_w_dedwv4Jjw7UorCREw5rXbRqIKhr","l":"http://auth.mybank/"}],"l":"http://mybank/","i64":"d2UgdXNlZCBvdXIgb3RoZXIgc2VjcmV0IGtleQ","s64":"0n2y_R8idg5MPa6BN-LY_B32wHQcGK7UuXJWv3jR9Vw"}`,
	expectVers: macaroon.V2,
}, {
	about:                "V2 object with URL base-64 binary values and strings for ASCII",
	data:                 `{"c":[{"i":"account = 3735928559"},{"i":"this was how we remind auth of key/pred","v64":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA027FAuBYhtHwJ58FX6UlVNFtFsGxQHS7uD_w_dedwv4Jjw7UorCREw5rXbRqIKhr","l":"http://auth.mybank/"}],"l":"http://mybank/","i":"we used our other secret key","s64":"0n2y_R8idg5MPa6BN-LY_B32wHQcGK7UuXJWv3jR9Vw"}`,
	expectVers:           macaroon.V2,
	expectExactRoundTrip: true,
}, {
	about: "V2 base64 encoded binary",
	data: `"` +
		base64.StdEncoding.EncodeToString([]byte(
			"\x02"+
				"\x01\x0ehttp://mybank/"+
				"\x02\x1cwe used our other secret key"+
				"\x00"+
				"\x02\x14account = 3735928559"+
				"\x00"+
				"\x01\x13http://auth.mybank/"+
				"\x02'this was how we remind auth of key/pred"+
				"\x04\x48\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xd3\x6e\xc5\x02\xe0\x58\x86\xd1\xf0\x27\x9f\x05\x5f\xa5\x25\x54\xd1\x6d\x16\xc1\xb1\x40\x74\xbb\xb8\x3f\xf0\xfd\xd7\x9d\xc2\xfe\x09\x8f\x0e\xd4\xa2\xb0\x91\x13\x0e\x6b\x5d\xb4\x6a\x20\xa8\x6b"+
				"\x00"+
				"\x00"+
				"\x06\x20\xd2\x7d\xb2\xfd\x1f\x22\x76\x0e\x4c\x3d\xae\x81\x37\xe2\xd8\xfc\x1d\xf6\xc0\x74\x1c\x18\xae\xd4\xb9\x72\x56\xbf\x78\xd1\xf5\x5c",
		)) + `"`,
	expectVers: macaroon.V2,
}}

func (s *macaroonSuite) TestJSONRoundTrip(c *gc.C) {
	for i, test := range jsonRoundTripTests {
		c.Logf("test %d (%v) %s", i, test.expectVers, test.about)
		s.testJSONRoundTripWithVersion(c, test.data, test.expectVers, test.expectExactRoundTrip)
	}
}

func (*macaroonSuite) testJSONRoundTripWithVersion(c *gc.C, jsonData string, vers macaroon.Version, expectExactRoundTrip bool) {
	var m macaroon.Macaroon
	err := json.Unmarshal([]byte(jsonData), &m)
	c.Assert(err, gc.IsNil)
	assertLibMacaroonsMacaroon(c, &m)
	c.Assert(m.Version(), gc.Equals, vers)

	data, err := m.MarshalJSON()
	c.Assert(err, gc.IsNil)

	if expectExactRoundTrip {
		// The data is in canonical form, so we can check that
		// the round-tripped data is the same as the original
		// data when unmarshalled into an interface{}.
		var got interface{}
		err = json.Unmarshal(data, &got)
		c.Assert(err, gc.IsNil)

		var original interface{}
		err = json.Unmarshal([]byte(jsonData), &original)
		c.Assert(err, gc.IsNil)

		c.Assert(got, jc.DeepEquals, original, gc.Commentf("data: %s", data))
	}
	// Check that we can unmarshal the marshaled data anyway
	// and the macaroon still looks the same.
	var m1 macaroon.Macaroon
	err = m1.UnmarshalJSON(data)
	c.Assert(err, gc.IsNil)
	assertLibMacaroonsMacaroon(c, &m1)
	c.Assert(m.Version(), gc.Equals, vers)
}

// assertLibMacaroonsMacaroon asserts that m looks like the macaroon
// created in the README of the libmacaroons documentation.
// In particular, the signature is the same one reported there.
func assertLibMacaroonsMacaroon(c *gc.C, m *macaroon.Macaroon) {
	c.Assert(fmt.Sprintf("%x", m.Signature()), gc.Equals,
		"d27db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c")
	c.Assert(m.Location(), gc.Equals, "http://mybank/")
	c.Assert(string(m.Id()), gc.Equals, "we used our other secret key")
	c.Assert(m.Caveats(), jc.DeepEquals, []macaroon.Caveat{{
		Id: []byte("account = 3735928559"),
	}, {
		Id:             []byte("this was how we remind auth of key/pred"),
		VerificationId: decodeB64("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA027FAuBYhtHwJ58FX6UlVNFtFsGxQHS7uD_w_dedwv4Jjw7UorCREw5rXbRqIKhr"),
		Location:       "http://auth.mybank/",
	}})
}

var jsonDecodeErrorTests = []struct {
	about       string
	data        string
	expectError string
}{{
	about:       "ambiguous id #1",
	data:        `{"i": "hello", "i64": "abcd", "s": "d27db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c"}`,
	expectError: "invalid identifier: ambiguous field encoding",
}, {
	about:       "ambiguous id #2",
	data:        `{"iH": "0000", "i64": "abcd", "s": "d27db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c"}`,
	expectError: "invalid identifier: ambiguous field encoding",
}, {
	about:       "ambiguous id #3",
	data:        `{"i": "hello", "iH": "a65b", "s": "d27db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c"}`,
	expectError: "invalid identifier: ambiguous field encoding",
}, {
	about:       "ambiguous signature",
	data:        `{"i": "hello", "sH": "d27db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c", "s64": "543467"}`,
	expectError: "invalid signature: ambiguous field encoding",
}, {
	about:       "signature too short",
	data:        `{"i": "hello", "sH": "7db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c"}`,
	expectError: "signature has unexpected length 31",
}, {
	about:       "signature too long",
	data:        `{"i": "hello", "sH": "00d27db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c"}`,
	expectError: "signature has unexpected length 33",
}, {
	about:       "invalid caveat id",
	data:        `{"i": "hello", "sH": "d27db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c", "c": [{"i": "hello", "iH": "00"}]}`,
	expectError: "invalid cid in caveat: ambiguous field encoding",
}, {
	about:       "invalid caveat vid",
	data:        `{"i": "hello", "sH": "d27db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c", "c": [{"i": "hello", "v": "hello", "vH": "00"}]}`,
	expectError: "invalid vid in caveat: ambiguous field encoding",
}}

func (*macaroonSuite) TestJSONDecodeError(c *gc.C) {
	for i, test := range jsonDecodeErrorTests {
		c.Logf("test %d: %v", i, test.about)
		var m macaroon.Macaroon
		err := json.Unmarshal([]byte(test.data), &m)
		c.Assert(err, gc.ErrorMatches, test.expectError)
	}
}

func decodeB64(s string) []byte {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return data
}

type caveat struct {
	rootKey   string
	location  string
	condition string
}

type macaroonSpec struct {
	rootKey  string
	id       string
	caveats  []caveat
	location string
}

func makeMacaroons(mspecs []macaroonSpec) (
	rootKey []byte,
	primary *macaroon.Macaroon,
	discharges []*macaroon.Macaroon,
) {
	var macaroons []*macaroon.Macaroon
	for _, mspec := range mspecs {
		macaroons = append(macaroons, makeMacaroon(mspec))
	}
	primary = macaroons[0]
	discharges = macaroons[1:]
	for _, m := range discharges {
		m.Bind(primary.Signature())
	}
	return []byte(mspecs[0].rootKey), primary, discharges
}

func makeMacaroon(mspec macaroonSpec) *macaroon.Macaroon {
	m := MustNew([]byte(mspec.rootKey), []byte(mspec.id), mspec.location, macaroon.LatestVersion)
	for _, cav := range mspec.caveats {
		if cav.location != "" {
			err := m.AddThirdPartyCaveat([]byte(cav.rootKey), []byte(cav.condition), cav.location)
			if err != nil {
				panic(err)
			}
		} else {
			m.AddFirstPartyCaveat(cav.condition)
		}
	}
	return m
}

func assertEqualMacaroons(c *gc.C, m0, m1 *macaroon.Macaroon) {
	m0json, err := m0.MarshalJSON()
	c.Assert(err, gc.IsNil)
	m1json, err := m1.MarshalJSON()
	var m0val, m1val interface{}
	err = json.Unmarshal(m0json, &m0val)
	c.Assert(err, gc.IsNil)
	err = json.Unmarshal(m1json, &m1val)
	c.Assert(err, gc.IsNil)
	c.Assert(m0val, gc.DeepEquals, m1val)
}

func (*macaroonSuite) TestBinaryRoundTrip(c *gc.C) {
	// Test the binary marshalling and unmarshalling of a macaroon with
	// first and third party caveats.
	rootKey := []byte("secret")
	m0 := MustNew(rootKey, []byte("some id"), "a location", macaroon.LatestVersion)
	err := m0.AddFirstPartyCaveat("first caveat")
	c.Assert(err, gc.IsNil)
	err = m0.AddFirstPartyCaveat("second caveat")
	c.Assert(err, gc.IsNil)
	err = m0.AddThirdPartyCaveat([]byte("shared root key"), []byte("3rd party caveat"), "remote.com")
	c.Assert(err, gc.IsNil)
	data, err := m0.MarshalBinary()
	c.Assert(err, gc.IsNil)
	var m1 macaroon.Macaroon
	err = m1.UnmarshalBinary(data)
	c.Assert(err, gc.IsNil)
	assertEqualMacaroons(c, m0, &m1)
}

func (*macaroonSuite) TestBinaryMarshalingAgainstLibmacaroon(c *gc.C) {
	// Test that a libmacaroon marshalled macaroon can be correctly unmarshaled
	data, err := base64.RawURLEncoding.DecodeString(
		"MDAxY2xvY2F0aW9uIGh0dHA6Ly9teWJhbmsvCjAwMmNpZGVudGlmaWVyIHdlIHVzZWQgb3VyIG90aGVyIHNlY3JldCBrZXkKMDAxZGNpZCBhY2NvdW50ID0gMzczNTkyODU1OQowMDMwY2lkIHRoaXMgd2FzIGhvdyB3ZSByZW1pbmQgYXV0aCBvZiBrZXkvcHJlZAowMDUxdmlkIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAANNuxQLgWIbR8CefBV-lJVTRbRbBsUB0u7g_8P3XncL-CY8O1KKwkRMOa120aiCoawowMDFiY2wgaHR0cDovL2F1dGgubXliYW5rLwowMDJmc2lnbmF0dXJlINJ9sv0fInYOTD2ugTfi2Pwd9sB0HBiu1LlyVr940fVcCg")
	c.Assert(err, gc.IsNil)
	var m0 macaroon.Macaroon
	err = m0.UnmarshalBinary(data)
	c.Assert(err, gc.IsNil)
	jsonData := []byte(`{"caveats":[{"cid":"account = 3735928559"},{"cid":"this was how we remind auth of key\/pred","vid":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA027FAuBYhtHwJ58FX6UlVNFtFsGxQHS7uD_w_dedwv4Jjw7UorCREw5rXbRqIKhr","cl":"http:\/\/auth.mybank\/"}],"location":"http:\/\/mybank\/","identifier":"we used our other secret key","signature":"d27db2fd1f22760e4c3dae8137e2d8fc1df6c0741c18aed4b97256bf78d1f55c"}`)
	var m1 macaroon.Macaroon
	err = m1.UnmarshalJSON(jsonData)
	c.Assert(err, gc.IsNil)
	assertEqualMacaroons(c, &m0, &m1)
}

func (*macaroonSuite) TestInvalidMacaroonFields(c *gc.C) {
	rootKey := []byte("secret")
	badString := "foo\xff"

	m0 := MustNew(rootKey, []byte("some id"), "a location", macaroon.LatestVersion)
	err := m0.AddFirstPartyCaveat(badString)
	c.Assert(err, gc.ErrorMatches, `first party caveat condition is not a valid utf-8 string`)
}
