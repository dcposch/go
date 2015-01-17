// A KeyFamily is a group of sibling keys that have equal power for a user.
// A family can consist of 1 PGP keys, and arbitrarily many NaCl Sibkeys.
// There also can be some subkeys dangling off for ECDH.
package libkb

import (
	"fmt"
	"github.com/keybase/go-jsonw"
)

// We have two notions of time we can use -- standard UTC which might
// be screwy (skewy) based upon local clock problems; or MerkleRoot seqno,
// which is totally ordered and all clients and server ought to agree on it.
// The issue is that we're not uniformly signing Merkle roots into signatures,
// especially those generated on the Web site.
type KeybaseTime struct {
	Unix  int64 // UTC wallclock time
	Chain int   // Merkle root chain time
}

type ComputedKeyInfo struct {
	Status      int
	Eldest      bool
	Sibkey      bool
	Delegations map[SigId]KID
	DelegatedAt *KeybaseTime
	RevokedAt   *KeybaseTime
}

// As returned by user/lookup.json
type ServerKeyRecord struct {
	Kid            string  `json:"kid"`
	KeyType        int     `json:"key_type"`
	Bundle         string  `json:"bundle"`
	Mtime          int     `json:"mtime"`
	Ctime          int     `json:"ctime"`
	Etime          int     `json:"etime"`
	PgpFingerprint string  `json:"key_fingerprint"`
	SigningKid     *string `json:"signing_kid"`
	EldestKid      *string `json:"eldest_kid"`
	KeyLevel       int     `json:"key_level"`
	Status         int     `json:"status"`
	KeyBits        int     `json:"key_bits"`
	KeyAlgo        int     `json:"key_algo"`

	key GenericKey `json:-`
}

type KeyMap map[string]ServerKeyRecord

// When we play a sigchain forward, it yields ComputedKeyInfos (CKIs). We're going to
// store CKIs separately from the keys, since the server can clobber the
// former.  We should rewrite CKIs every time we (re)check a user's SigChain
type ComputedKeyInfos struct {
	dirty bool // whether it needs to be written to disk or not

	// Map of KID (in HEX) to a computed info
	Infos map[string]*ComputedKeyInfo

	// Map of a SigId (in Binary) to the ComputedKeyInfo describing when the key was
	// delegated.
	Sigs map[SigId]*ComputedKeyInfo
}

// As returned by user/lookup.json
type KeyFamily struct {
	eldest  *FOKID
	pgps    []*PgpKeyBundle
	pgp2kid map[string]KID

	Sibkeys KeyMap `json:"sibkeys"`
	Subkeys KeyMap `json:"subkeys"`
}

type ComputedKeyFamily struct {
	kf  *KeyFamily
	cki *ComputedKeyInfos
}

// Insert inserts the given ComputedKeyInfo object 1 or 2 times,
// depending on if a KID or PgpFingerprint or both are available.
func (cki *ComputedKeyInfos) Insert(f *FOKID, i *ComputedKeyInfo) {
	if f != nil {
		v := f.ToStrings()
		for _, s := range v {
			cki.Infos[s] = i
		}
		cki.dirty = true
	}
}

// NewComputedKeyInfos creates a new ComputedKeyInfos object
// from the given key family.  It finds the eldest sibling in the family and marks
// his status LIVE, so that he can be used to verify signatures. There's
// obviously no one who can delegate to him, so we take it on faith.
func (kf KeyFamily) NewComputedKeyInfos() *ComputedKeyInfos {

	ret := ComputedKeyInfos{
		Infos: make(map[string]*ComputedKeyInfo),
	}

	ret.Insert(kf.eldest, &ComputedKeyInfo{
		Eldest: true,
		Status: KEY_LIVE,
	})

	return &ret
}

// FindSibkey finds a sibkey in our KeyFamily, by either PGP fingerprint or
// KID. It returns the GenericKey object that's useful for actually performing
// PGP ops.
func (kf KeyFamily) FindActiveSibkey(f FOKID) (key GenericKey, err error) {

	var found bool
	var i string
	kid := f.Kid
	if kid == nil && f.Fp != nil {
		i = f.Fp.ToString()
		if kid, found = kf.pgp2kid[i]; !found {
			err = NoKeyError{fmt.Sprintf("No KID for PGP fingerprint %s found", i)}
			return
		}
	}
	if kid == nil {
		err = NoKeyError{"Can't lookup sibkey without a KID"}
		return
	}

	i = f.Kid.ToString()
	if sk, ok := kf.Sibkeys[i]; !ok {
		err = NoKeyError{fmt.Sprintf("No sibkey found for %s", i)}
	} else {
		key = sk.key
	}
	return
}

// Import takes all ServerKeyRecords in this KeyMap and imports the
// key bundle into a GenericKey object that can perform crypto ops. It
// also collects all PgpKeyBundles along the way.
func (km KeyMap) Import(pgps_i []*PgpKeyBundle) (pgps_o []*PgpKeyBundle, err error) {
	pgps_o = pgps_i
	for _, v := range km {
		var pgp *PgpKeyBundle
		if pgp, err = v.Import(); err != nil {
			return
		}
		if pgp != nil {
			pgps_o = append(pgps_o, pgp)
		}
	}
	return
}

// Import takes all Subkeys and Subkeys and imports them and indexes them.
// It indexes them both by KID and by PgpFingerprint, if available.
func (kf *KeyFamily) Import() (err error) {
	G.Log.Debug("+ ImportKeys")
	defer func() {
		G.Log.Debug("- ImportKeys -> %s", ErrToOk(err))
	}()
	if kf.pgps, err = kf.Sibkeys.Import(kf.pgps); err != nil {
		return
	}
	if kf.pgps, err = kf.Subkeys.Import(kf.pgps); err != nil {
		return
	}
	for _, p := range kf.pgps {
		kf.pgp2kid[p.GetFingerprint().ToString()] = p.GetKid()
	}
	err = kf.findEldest()
	return
}

// setEldest sets this keyFamily's eldest KID to the given KID (specified in hex).
// It is strict that there can only be one eldest KID in the family.
func (kf *KeyFamily) setEldest(hx string) (err error) {
	var kid KID
	if kid, err = ImportKID(hx); err != nil {
		return
	}
	if kf.eldest == nil {
		kf.eldest = &FOKID{Kid: kid}
	} else if !kf.eldest.EqKid(kid) {
		err = KeyFamilyError{fmt.Sprintf("Kid mismatch: %s != %s",
			kf.eldest.Kid.ToString(), hx)}
	}
	return
}

// GetEldest gets the KID of the eldest key in the family.
func (kf *KeyFamily) GetEldest() *FOKID {
	return kf.eldest
}

// findEldest finds the eldest key in the given Key family, and sanity
// checks that the eldest key is unique.  If tests pass, it sets a "FOKID"
// object to capture both the KID and the (optional) PgpFingerprint
// of the eldest key in the family.
func (kf *KeyFamily) findEldest() (err error) {
	for _, v := range kf.Sibkeys {
		if v.EldestKid == nil {
			err = kf.setEldest(v.Kid)
		} else {
			err = kf.setEldest(*v.EldestKid)
		}
		if err != nil {
			return
		}
	}
	if kf.eldest != nil {
		x := kf.eldest.Kid.ToString()
		if key, found := kf.Sibkeys[x]; !found {
			err = KeyFamilyError{fmt.Sprintf("Eldest KID %s disappeared", x)}
		} else {
			kf.eldest.Fp, err = PgpFingerprintFromHex(key.PgpFingerprint)
		}
	}
	return
}

// ParseKeyFamily takes as input a dictionary from a JSON file and returns
// a parsed version for manipulation in the program.
func ParseKeyFamily(jw *jsonw.Wrapper) (ret *KeyFamily, err error) {
	if jw == nil && jw.IsNil() {
		err = KeyFamilyError{"nil record from server"}
	}

	// Somewhat wasteful but probably faster than using Jsonw wrappers,
	// and less error-prone
	var obj KeyFamily
	if err = jw.UnmarshalAgain(&obj); err != nil {
		return
	}
	if err = obj.Import(); err != nil {
		return
	}
	ret = &obj
	return
}

func (skr *ServerKeyRecord) Import() (pgp *PgpKeyBundle, err error) {
	switch {
	case IsPgpAlgo(skr.KeyAlgo):
		if pgp, err = ReadOneKeyFromString(skr.Bundle); err == nil {
			skr.key = pgp
		}
	case skr.KeyAlgo == KID_NACL_EDDSA:
		skr.key, err = ImportNaclSigningKeyPair(skr.Bundle)
	case skr.KeyAlgo == KID_NACL_DH:
		skr.key, err = ImportNaclDHKeyPair(skr.Bundle)
	default:
		err = BadKeyError{fmt.Sprintf("algo=%d is unknown", skr.KeyAlgo)}
	}
	if err == nil {
		G.Log.Debug("| Imported Key %s", skr.key.GetKid().ToString())
	}
	return
}

func (kf KeyFamily) GetSigningKey(kid_s string) (ret GenericKey) {
	if skr, found := kf.Sibkeys[kid_s]; found {
		ret = skr.key
	}
	return
}

func (ckf ComputedKeyFamily) FindActiveSibkey(f FOKID) (key GenericKey, err error) {
	s := f.ToString()
	if ki := ckf.cki.Infos[s]; ki == nil {
		err = NoKeyError{fmt.Sprintf("The key '%s' wasn't found", s)}
	} else if ki.Status != KEY_LIVE {
		err = BadKeyError{fmt.Sprintf("The key '%s' is no longer active", s)}
	} else if !ki.Sibkey {
		err = BadKeyError{fmt.Sprintf("The key '%s' wasn't delegated as a sibkey", s)}
	} else {
		key, err = ckf.FindActiveSibkey(f)
	}
	return
}

// TclToKeybaseTime turns a TypedChainLink into a KeybaseTime tuple, looking
// inside the chainlink for the Unix wallclock and the global MerkleChain seqno.
func TclToKeybaseTime(tcl TypedChainLink) *KeybaseTime {
	return &KeybaseTime{
		Unix:  tcl.GetCTime().Unix(),
		Chain: tcl.GetMerkleSeqno(),
	}
}

// Delegate performs a delegation to the key described in the given TypedChainLink.
// This maybe be a sub- or sibkey delegation.
func (ckf *ComputedKeyFamily) Delegate(tcl TypedChainLink) (err error) {
	kid := tcl.GetDelegatedKid()
	kid_s := kid.ToString()
	sigid := tcl.GetSigId()
	tm := TclToKeybaseTime(tcl)

	info, found := ckf.cki.Infos[kid_s]
	if !found {
		info = &ComputedKeyInfo{
			Eldest:      false,
			Status:      KEY_LIVE,
			Delegations: make(map[SigId]KID),
			DelegatedAt: tm,
		}
		ckf.cki.Infos[kid_s] = info
	} else {
		info.Status = KEY_LIVE
	}
	info.Delegations[sigid] = tcl.GetKid()
	if tcl.IsDelegation() == DLG_SIBKEY {
		info.Sibkey = true
	}
	ckf.cki.Sigs[sigid] = info
	return
}

func (ckf *ComputedKeyFamily) Revoke(tcl TypedChainLink) (err error) {
	err = ckf.RevokeSigs(tcl.GetRevocations(), tcl)
	if err == nil {
		err = ckf.RevokeKids(tcl.GetRevokeKids(), tcl)
	}
	return err
}

func (ckf *ComputedKeyFamily) RevokeSigs(sigs []*SigId, tcl TypedChainLink) (err error) {
	for _, s := range sigs {
		if s != nil {
			if err = ckf.RevokeSig(*s, tcl); err != nil {
				return
			}
		}
	}
	return
}

func (ckf *ComputedKeyFamily) RevokeKids(kids []KID, tcl TypedChainLink) (err error) {
	for _, k := range kids {
		if k != nil {
			if err = ckf.RevokeKid(k, tcl); err != nil {
				return
			}
		}
	}
	return
}

func (ckf *ComputedKeyFamily) RevokeSig(sig SigId, tcl TypedChainLink) (err error) {
	if info, found := ckf.cki.Sigs[sig]; !found {
	} else if _, found = info.Delegations[sig]; !found {
		err = BadRevocationError{fmt.Sprintf("Can't find sigId %s in delegation list",
			sig.ToString(true))}
	} else {
		info.Status = KEY_REVOKED
		info.RevokedAt = TclToKeybaseTime(tcl)
	}
	return
}

func (ckf *ComputedKeyFamily) RevokeKid(kid KID, tcl TypedChainLink) (err error) {
	if info, found := ckf.cki.Infos[kid.ToString()]; found {
		info.Status = KEY_REVOKED
		info.RevokedAt = TclToKeybaseTime(tcl)
	}
	return
}

// FindKeybaseName looks at all PGP keys in this key family that are active
// sibkeys to find a key with a signed identity of <name@keybase.io>. IF
// found return true, and otherwise false.
func (ckf ComputedKeyFamily) FindKeybaseName(s string) bool {
	kem := KeybaseEmailAddress(s)
	for _, pgp := range ckf.kf.pgps {
		kid := pgp.GetKid()
		if info, found := ckf.cki.Infos[kid.ToString()]; !found {
			continue
		} else if info.Status != KEY_LIVE || !info.Sibkey {
			continue
		}
		if pgp.FindEmail(kem) {
			G.Log.Debug("| Found self-sig for %s in key ID: %s", s, kid.ToString())
			return true
		}
	}
	return false
}
