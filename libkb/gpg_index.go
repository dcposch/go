package libkb

import (
	"bufio"
	"fmt"
	"golang.org/x/crypto/openpgp/packet"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

//=============================================================================

type BucketDict struct {
	d map[string][]*GpgPrimaryKey
}

func NewBuckDict() *BucketDict {
	return &BucketDict{
		d: make(map[string][]*GpgPrimaryKey),
	}
}

func (bd *BucketDict) Add(k string, v *GpgPrimaryKey) {
	k = strings.ToLower(k)
	var bucket []*GpgPrimaryKey
	var found bool
	if bucket, found = bd.d[k]; !found {
		bucket = make([]*GpgPrimaryKey, 0, 1)
	}
	bucket = append(bucket, v)
	bd.d[k] = bucket
}

func (bd BucketDict) Get(k string) []*GpgPrimaryKey {
	k = strings.ToLower(k)
	ret, found := bd.d[k]
	if !found {
		ret = nil
	}
	return ret
}

func (bd BucketDict) Get0Or1(k string) (ret *GpgPrimaryKey, err error) {
	v := bd.Get(k)
	if len(v) > 1 {
		err = GpgError{fmt.Sprintf("Wanted a unique lookup but got %d objects for key %s", len(v), k)}
	} else if len(v) == 1 {
		ret = v[0]
	}
	return
}

//=============================================================================

func Uniquify(inp []string) []string {
	m := make(map[string]bool)
	for _, s := range inp {
		m[strings.ToLower(s)] = true
	}
	ret := make([]string, 0, len(inp))
	for k := range m {
		ret = append(ret, k)
	}
	return ret
}

//=============================================================================

type GpgBaseKey struct {
	Type        string
	Trust       string
	Bits        int
	Algo        int
	Id64        string
	Created     int
	Expires     int
	fingerprint *PgpFingerprint
}

func (k GpgBaseKey) AlgoString() string {
	switch packet.PublicKeyAlgorithm(k.Algo) {
	case packet.PubKeyAlgoDSA:
		return "D"
	case packet.PubKeyAlgoRSA:
		return "R"
	case packet.PubKeyAlgoECDSA:
		return "E"
	default:
		return "?"
	}
}

func (k GpgBaseKey) ExpirationString() string {
	if k.Expires == 0 {
		return "never"
	} else {
		layout := "2006-01-02"
		return time.Unix(int64(k.Expires), 0).Format(layout)
	}
}

func (k *GpgBaseKey) ParseBase(line *GpgIndexLine) (err error) {
	if line.Len() < 12 {
		err = GpgIndexError{line.lineno, "Not enough fields (need 12)"}
		return
	}

	k.Type = line.At(0)
	k.Trust = line.At(1)
	k.Id64 = line.At(4)

	flexiAtoi := func(s string) (int, error) {
		if len(s) == 0 {
			return 0, nil
		} else {
			return strconv.Atoi(s)
		}
	}

	if k.Bits, err = strconv.Atoi(line.At(2)); err != nil {
	} else if k.Algo, err = strconv.Atoi(line.At(3)); err != nil {
	} else if k.Created, err = strconv.Atoi(line.At(5)); err != nil {
	} else if k.Expires, err = flexiAtoi(line.At(6)); err != nil {
	}

	return
}

//=============================================================================

type GpgFingerprinter interface {
	SetFingerprint(pgp *PgpFingerprint)
}

type GpgPrimaryKey struct {
	GpgBaseKey
	subkeys    []*GpgSubKey
	identities []*Identity
	top        GpgFingerprinter
}

func (k *GpgPrimaryKey) IsValid() bool {
	if k.Trust == "r" {
		return false
	} else if k.Expires == 0 {
		return true
	} else {
		return time.Now().Before(time.Unix(int64(k.Expires), 0))
	}
}

func (k *GpgPrimaryKey) ToRow(i int) []string {
	v := []string{
		fmt.Sprintf("(%d)", i),
		fmt.Sprintf("%d%s", k.Bits, k.AlgoString()),
		k.fingerprint.ToKeyId(),
		k.ExpirationString(),
	}
	for _, i := range k.identities {
		v = append(v, i.Email)
	}
	return v
}

func (k *GpgBaseKey) SetFingerprint(pgp *PgpFingerprint) {
	k.fingerprint = pgp
}

func (k *GpgPrimaryKey) Parse(l *GpgIndexLine) (err error) {
	if err = k.ParseBase(l); err != nil {
	} else if err = k.AddUid(l); err != nil {
	}
	return
}

func NewGpgPrimaryKey() *GpgPrimaryKey {
	ret := &GpgPrimaryKey{}
	ret.top = ret
	return ret
}

func ParseGpgPrimaryKey(l *GpgIndexLine) (key *GpgPrimaryKey, err error) {
	key = NewGpgPrimaryKey()
	err = key.Parse(l)
	return
}

func (k *GpgPrimaryKey) AddUid(l *GpgIndexLine) (err error) {
	var id *Identity
	if f := l.At(9); len(f) == 0 {
	} else if id, err = ParseIdentity(f); err != nil {
	} else {
		k.identities = append(k.identities, id)
	}
	if err != nil {
		err = ErrorToGpgIndexError(l.lineno, err)
	}
	return
}

func (k *GpgPrimaryKey) AddFingerprint(l *GpgIndexLine) (err error) {
	var fp *PgpFingerprint
	if f := l.At(9); len(f) == 0 {
		err = fmt.Errorf("no fingerprint given")
	} else if fp, err = PgpFingerprintFromHex(f); err == nil {
		k.top.SetFingerprint(fp)
	}
	if err != nil {
		err = ErrorToGpgIndexError(l.lineno, err)
	}
	return
}

func (k *GpgPrimaryKey) GetFingerprint() *PgpFingerprint {
	return k.fingerprint
}

func (k *GpgPrimaryKey) GetEmails() []string {
	ret := make([]string, 0, len(k.identities))
	for _, i := range k.identities {
		ret = append(ret, i.Email)
	}
	return ret
}

func (k *GpgPrimaryKey) GetAllId64s() []string {
	var ret []string
	add := func(fp *PgpFingerprint) {
		if fp != nil {
			ret = append(ret, fp.ToKeyId())
		}
	}
	add(k.GetFingerprint())
	for _, sk := range k.subkeys {
		add(sk.fingerprint)
	}
	return ret
}

func (g *GpgPrimaryKey) AddSubkey(l *GpgIndexLine) (err error) {
	var sk *GpgSubKey
	if sk, err = ParseGpgSubKey(l); err == nil {
		g.subkeys = append(g.subkeys, sk)
		g.top = sk
	}
	return
}

func (g *GpgPrimaryKey) ToKey() *GpgPrimaryKey { return g }

func (p *GpgPrimaryKey) AddLine(l *GpgIndexLine) (err error) {
	if l.Len() < 2 {
		err = GpgIndexError{l.lineno, "too few fields"}
	} else {
		f := l.At(0)
		switch f {
		case "fpr":
			err = p.AddFingerprint(l)
		case "uid":
			err = p.AddUid(l)
		case "uat":
		case "sub", "ssb":
			err = p.AddSubkey(l)
		default:
			err = GpgIndexError{l.lineno, fmt.Sprintf("Unknown subfield: %s", f)}
		}

	}
	return err
}

//=============================================================================

type GpgSubKey struct {
	GpgBaseKey
}

func ParseGpgSubKey(l *GpgIndexLine) (sk *GpgSubKey, err error) {
	sk = &GpgSubKey{}
	err = sk.ParseBase(l)
	return
}

//=============================================================================

type GpgIndexElement interface {
	ToKey() *GpgPrimaryKey
}

type GpgKeyIndex struct {
	Keys                        []*GpgPrimaryKey
	Emails, Fingerprints, Id64s *BucketDict
}

func (ki *GpgKeyIndex) Len() int {
	return len(ki.Keys)
}
func (ki *GpgKeyIndex) Swap(i, j int) {
	ki.Keys[i], ki.Keys[j] = ki.Keys[j], ki.Keys[i]
}
func (ki *GpgKeyIndex) Less(i, j int) bool {
	a, b := ki.Keys[i], ki.Keys[j]
	if len(a.identities) > len(b.identities) {
		return true
	} else if len(a.identities) < len(b.identities) {
		return false
	} else if a.Expires == 0 {
		return true
	} else if b.Expires == 0 {
		return false
	} else if a.Expires > b.Expires {
		return true
	} else {
		return false
	}
}

func (p *GpgKeyIndex) GetRowFunc() func() []string {
	i := 0
	return func() []string {
		if i >= len(p.Keys) {
			return nil
		} else {
			ret := p.Keys[i].ToRow(i + 1)
			i++
			return ret
		}
	}
}

func (ki *GpgKeyIndex) Sort() {
	sort.Sort(ki)
}

func NewGpgKeyIndex() *GpgKeyIndex {
	return &GpgKeyIndex{
		Keys:         make([]*GpgPrimaryKey, 0, 1),
		Emails:       NewBuckDict(),
		Fingerprints: NewBuckDict(),
		Id64s:        NewBuckDict(),
	}
}

func (ki *GpgKeyIndex) IndexKey(k *GpgPrimaryKey) {
	ki.Keys = append(ki.Keys, k)
	if fp := k.GetFingerprint(); fp != nil {
		ki.Fingerprints.Add(fp.String(), k)
	}
	for _, e := range Uniquify(k.GetEmails()) {
		ki.Emails.Add(e, k)
	}
	for _, i := range Uniquify(k.GetAllId64s()) {
		ki.Id64s.Add(i, k)
	}
}

func (k *GpgKeyIndex) PushElement(e GpgIndexElement) {
	if key := e.ToKey(); key == nil {
	} else if key.IsValid() {
		k.IndexKey(key)
	}
}

func (ki *GpgKeyIndex) AllFingerprints() []PgpFingerprint {
	ret := make([]PgpFingerprint, 0, 1)
	for _, k := range ki.Keys {
		if fp := k.GetFingerprint(); fp != nil {
			ret = append(ret, *fp)
		}
	}
	return ret
}

//=============================================================================

type GpgIndexLine struct {
	v      []string
	lineno int
}

func (g GpgIndexLine) Len() int        { return len(g.v) }
func (g GpgIndexLine) At(i int) string { return g.v[i] }

func ParseLine(s string, i int) (ret *GpgIndexLine, err error) {
	s = strings.TrimSpace(s)
	v := strings.Split(s, ":")
	if v == nil {
		err = GpgError{fmt.Sprintf("%d: Bad line; split failed", i)}
	} else {
		ret = &GpgIndexLine{v, i}
	}
	return
}

func (l GpgIndexLine) IsNewKey() bool {
	return len(l.v) > 0 && (l.v[0] == "sec" || l.v[0] == "pub")
}

//=============================================================================

type GpgIndexParser struct {
	warnings Warnings
	putback  *GpgIndexLine
	src      *bufio.Reader
	eof      bool
	lineno   int
}

func NewGpgIndexParser() *GpgIndexParser {
	return &GpgIndexParser{
		eof:     false,
		lineno:  0,
		putback: nil,
	}
}

func (p *GpgIndexParser) Warn(w Warning) {
	p.warnings.Push(w)
}

func (p *GpgIndexParser) ParseElement() (ret GpgIndexElement, err error) {
	var line *GpgIndexLine
	line, err = p.GetLine()
	if err != nil || line == nil {
	} else if line.IsNewKey() {
		ret, err = p.ParseKey(line)
	}
	return
}

func (p *GpgIndexParser) ParseKey(l *GpgIndexLine) (ret *GpgPrimaryKey, err error) {
	var line *GpgIndexLine
	ret, err = ParseGpgPrimaryKey(l)
	done := false
	for !done && err == nil && !p.isEof() {
		if line, err = p.GetLine(); line == nil || err != nil {
		} else if line.IsNewKey() {
			p.PutbackLine(line)
			done = true
		} else if e2 := ret.AddLine(line); e2 == nil {
		} else {
			p.warnings.Push(ErrorToWarning(e2))
		}
	}
	return
}

func (p *GpgIndexParser) GetLine() (ret *GpgIndexLine, err error) {
	if p.putback != nil {
		ret = p.putback
		p.putback = nil
	} else if p.isEof() {
	} else if s, e2 := p.src.ReadString(byte('\n')); e2 == nil {
		p.lineno++
		ret, err = ParseLine(s, p.lineno)
	} else if e2 == io.EOF {
		p.eof = true
	} else {
		err = e2
	}
	return
}

func (p *GpgIndexParser) PutbackLine(line *GpgIndexLine) {
	p.putback = line
}

func (p GpgIndexParser) isEof() bool { return p.eof }

func (p *GpgIndexParser) Parse(stream io.Reader) (ki *GpgKeyIndex, err error) {
	p.src = bufio.NewReader(stream)
	ki = NewGpgKeyIndex()
	for err == nil && !p.isEof() {
		var el GpgIndexElement
		if el, err = p.ParseElement(); err == nil && el != nil {
			ki.PushElement(el)
		}
	}
	ki.Sort()
	return
}

//=============================================================================

func ParseGpgIndexStream(stream io.Reader) (ki *GpgKeyIndex, err error, w Warnings) {
	eng := NewGpgIndexParser()
	ki, err = eng.Parse(stream)
	w = eng.warnings
	return
}

//=============================================================================

func (g *GpgCLI) Index(secret bool, query string) (ki *GpgKeyIndex, err error, w Warnings) {
	var k string
	if secret {
		k = "-K"
	} else {
		k = "-k"
	}
	args := []string{"--with-colons", "--fingerprint", k}
	if len(query) > 0 {
		args = append(args, query)
	}
	garg := RunGpg2Arg{
		Arguments: args,
		Stdout:    true,
	}
	if res := g.Run2(garg); res.Err != nil {
		err = res.Err
	} else if ki, err, w = ParseGpgIndexStream(res.Stdout); err != nil {
	} else {
		err = res.Wait()
	}
	return
}

//=============================================================================
