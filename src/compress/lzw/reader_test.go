// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzw

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type lzwTest struct {
	desc       string
	raw        string
	compressed string
	err        error
}

var lzwTests = []lzwTest{
	{
		"empty;LSB;8",
		"",
		"\x01\x01",
		nil,
	},
	{
		"empty;MSB;8",
		"",
		"\x80\x80",
		nil,
	},
	{
		"tobe;LSB;7",
		"TOBEORNOTTOBEORTOBEORNOT",
		"\x54\x4f\x42\x45\x4f\x52\x4e\x4f\x54\x82\x84\x86\x8b\x85\x87\x89\x81",
		nil,
	},
	{
		"tobe;LSB;8",
		"TOBEORNOTTOBEORTOBEORNOT",
		"\x54\x9e\x08\x29\xf2\x44\x8a\x93\x27\x54\x04\x12\x34\xb8\xb0\xe0\xc1\x84\x01\x01",
		nil,
	},
	{
		"tobe;MSB;7",
		"TOBEORNOTTOBEORTOBEORNOT",
		"\x54\x4f\x42\x45\x4f\x52\x4e\x4f\x54\x82\x84\x86\x8b\x85\x87\x89\x81",
		nil,
	},
	{
		"tobe;MSB;8",
		"TOBEORNOTTOBEORTOBEORNOT",
		"\x2a\x13\xc8\x44\x52\x79\x48\x9c\x4f\x2a\x40\xa0\x90\x68\x5c\x16\x0f\x09\x80\x80",
		nil,
	},
	{
		"tobe-truncated;LSB;8",
		"TOBEORNOTTOBEORTOBEORNOT",
		"\x54\x9e\x08\x29\xf2\x44\x8a\x93\x27\x54\x04",
		io.ErrUnexpectedEOF,
	},
	// This example comes from http://en.wikipedia.org/wiki/Graphics_Interchange_Format.
	{
		"gif;LSB;8",
		"\x28\xff\xff\xff\x28\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff",
		"\x00\x51\xfc\x1b\x28\x70\xa0\xc1\x83\x01\x01",
		nil,
	},
	// This example comes from http://compgroups.net/comp.lang.ruby/Decompressing-LZW-compression-from-PDF-file
	{
		"pdf;MSB;8",
		"-----A---B",
		"\x80\x0b\x60\x50\x22\x0c\x0c\x85\x01",
		nil,
	},
}

func TestReader(t *testing.T) {
	var b bytes.Buffer
	for _, tt := range lzwTests {
		d := strings.Split(tt.desc, ";")
		var order Order
		switch d[1] {
		case "LSB":
			order = LSB
		case "MSB":
			order = MSB
		default:
			t.Errorf("%s: bad order %q", tt.desc, d[1])
		}
		litWidth, _ := strconv.Atoi(d[2])
		rc := NewReader(strings.NewReader(tt.compressed), order, litWidth)
		defer rc.Close()
		b.Reset()
		n, err := io.Copy(&b, rc)
		s := b.String()
		if err != nil {
			if err != tt.err {
				t.Errorf("%s: io.Copy: %v want %v", tt.desc, err, tt.err)
			}
			if err == io.ErrUnexpectedEOF {
				// Even if the input is truncated, we should still return the
				// partial decoded result.
				if n == 0 || !strings.HasPrefix(tt.raw, s) {
					t.Errorf("got %d bytes (%q), want a non-empty prefix of %q", n, s, tt.raw)
				}
			}
			continue
		}
		if s != tt.raw {
			t.Errorf("%s: got %d-byte %q want %d-byte %q", tt.desc, n, s, len(tt.raw), tt.raw)
		}
	}
}

type devZero struct{}

func (devZero) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestHiCodeDoesNotOverflow(t *testing.T) {
	r := NewReader(devZero{}, LSB, 8)
	d := r.(*decoder)
	buf := make([]byte, 1024)
	oldHi := uint16(0)
	for i := 0; i < 100; i++ {
		if _, err := io.ReadFull(r, buf); err != nil {
			t.Fatalf("i=%d: %v", i, err)
		}
		// The hi code should never decrease.
		if d.hi < oldHi {
			t.Fatalf("i=%d: hi=%d decreased from previous value %d", i, d.hi, oldHi)
		}
		oldHi = d.hi
	}
}

func BenchmarkDecoder(b *testing.B) {
	buf, err := ioutil.ReadFile("../testdata/e.txt")
	if err != nil {
		b.Fatal(err)
	}
	if len(buf) == 0 {
		b.Fatalf("test file has no data")
	}

	for e := 4; e <= 6; e++ {
		n := int(math.Pow10(e))
		b.Run(fmt.Sprint("1e", e), func(b *testing.B) {
			b.StopTimer()
			b.SetBytes(int64(n))
			buf0 := buf
			compressed := new(bytes.Buffer)
			w := NewWriter(compressed, LSB, 8)
			for i := 0; i < n; i += len(buf0) {
				if len(buf0) > n-i {
					buf0 = buf0[:n-i]
				}
				w.Write(buf0)
			}
			w.Close()
			buf1 := compressed.Bytes()
			buf0, compressed, w = nil, nil, nil
			runtime.GC()
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				io.Copy(ioutil.Discard, NewReader(bytes.NewReader(buf1), LSB, 8))
			}
		})
	}
}
