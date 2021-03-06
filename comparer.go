package achile

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	Deleted   = 'D'
	Identical = 'I'
	Modified  = 'M'
	Added     = 'A'
)

type Comparer struct {
	digest *Digest

	pretty  bool
	verbose bool

	inner *bufio.Reader
	io.Closer
}

func NewComparer(file string, opts ...Option) (*Comparer, error) {
	r, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	var (
		buf = make([]byte, 16)
		alg string
	)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	alg = string(bytes.Trim(buf, "\x00"))

	var c Comparer
	if c.digest, err = NewDigest(alg); err != nil {
		return nil, err
	}
	c.inner = bufio.NewReader(r)
	c.Closer = r

	for _, o := range opts {
		o(&c)
	}

	return &c, nil
}

func (c *Comparer) List(dirs []string) (Coze, error) {
	for i := range dirs {
		dirs[i] = filepath.Clean(dirs[i])
	}
	var cz Coze
	for i := range FetchInfos(c.inner, c.digest.Size()) {
		fi, found := c.lookupFile(i, dirs)
		if !found {
			return cz, fmt.Errorf("%s: no such file", fi.File)
		}
		if c.verbose {
			if c.pretty {
				fmt.Printf("%-8s  %x  %s\n", FormatSize(fi.Size), c.digest.Local(), fi.File)
			} else {
				fmt.Printf("%-12d  %x  %s\n", int64(fi.Size), c.digest.Local(), fi.File)
			}
		}
		cz.Update(fi.Size)
	}
	return cz, nil
}

func (c *Comparer) Compare(dirs []string) (Coze, error) {
	for i := range dirs {
		dirs[i] = filepath.Clean(dirs[i])
	}
	cz, err := c.compareFiles(dirs)
	if err == nil {
		_, err = c.compare(cz)
	}
	return cz, err
}

func (c *Comparer) Checksum() []byte {
	return c.digest.Global()
}

func (c *Comparer) compareFiles(dirs []string) (Coze, error) {
	var (
		cz Coze
		st byte
	)
	for i := range FetchInfos(c.inner, c.digest.Size()) {
		fi, found := c.lookupFile(i, dirs)
		if found {
			st = Identical
			if err := c.digestFile(fi); err != nil {
				st = Modified
			}
			cz.Update(fi.Size)
		} else {
			st = Deleted
		}
		if c.verbose {
			if c.pretty {
				fmt.Printf("%c  %-8s  %x  %s\n", st, FormatSize(fi.Size), c.digest.Local(), fi.File)
			} else {
				fmt.Printf("%c  %-12d  %x  %s\n", st, int64(fi.Size), c.digest.Local(), fi.File)
			}
		}
		c.digest.Reset()
	}
	return cz, nil
}

func (c *Comparer) compare(cz Coze) (Coze, error) {
	var z Coze
	binary.Read(c.inner, binary.BigEndian, &z.Count)
	binary.Read(c.inner, binary.BigEndian, &z.Size)
	if !cz.Equal(z) {
		return z, fmt.Errorf("final count/size mismatched!")
	}

	accu := make([]byte, c.digest.Size())
	if _, err := io.ReadFull(c.inner, accu); err != nil {
		return cz, err
	}
	if sum := c.digest.Global(); !bytes.Equal(sum, accu) {
		return z, fmt.Errorf("final checksum mismatched (%x != %x!)", sum, accu)
	}
	return z, nil
}

func (c *Comparer) lookupFile(fi FileInfo, dirs []string) (FileInfo, bool) {
	var found bool
	for _, d := range dirs {
		file := filepath.Join(d, fi.File)
		if s, err := os.Stat(file); err == nil && s.Mode().IsRegular() {
			fi.File, found = file, true
			break
		}
	}
	return fi, found
}

func (c *Comparer) digestFile(fi FileInfo) error {
	r, err := os.Open(fi.File)
	if err != nil {
		return err
	}
	defer r.Close()

	n, err := io.Copy(c.digest, r)
	if err != nil {
		return err
	}
	if n != int64(fi.Size) {
		return fmt.Errorf("%s: size mismatched (%f != %d)!", fi.Size, fi.Size, n)
	}
	if sum := c.digest.Local(); !bytes.Equal(fi.Curr, sum) {
		return fmt.Errorf("%s: checksum mismatched (%x != %x)!", fi.File, fi.Curr, sum)
	}
	return nil
}

func (c *Comparer) setVerbose(v bool) { c.verbose = v }

func (c *Comparer) setPretty(v bool) { c.pretty = v }

func (c *Comparer) setError(v bool) {}
