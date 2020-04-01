package config_loader

import (
	"io/ioutil"
	"os"

	. "gopkg.in/check.v1"

	. "godropbox/gocheck2"
)

type StringProvider struct {
}

func (p *StringProvider) Default() interface{} {
	return ""
}

func (p *StringProvider) Parse(content []byte) (cfg interface{}, err error) {
	return string(content), nil
}

func (p *StringProvider) Validate(cfg interface{}) error {
	return nil
}

func (p *StringProvider) Equals(cfg1 interface{}, cfg2 interface{}) bool {
	return cfg1.(string) == cfg2.(string)
}

type OneTimeFileLoaderSuite struct {
}

var _ = Suite(&OneTimeFileLoaderSuite{})

func (l *OneTimeFileLoaderSuite) TestLoader(c *C) {
	content := []byte("test content")
	tmpfile, err := ioutil.TempFile("", "test.*.txt")
	c.Assert(err, IsNil)
	defer os.Remove(tmpfile.Name()) // clean up

	_, err = tmpfile.Write(content)
	c.Assert(err, IsNil)
	err = tmpfile.Close()
	c.Assert(err, IsNil)

	loader, err := NewOneTimeFileLoader(&StringProvider{}, tmpfile.Name())
	c.Assert(err, IsNil)

	select {
	case fileContent, ok := <-loader.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(fileContent, Equals, string(content))
	default:
		c.Fail()
	}
}

func (l *OneTimeFileLoaderSuite) TestMissedFile(c *C) {
	loader, err := NewOneTimeFileLoader(&StringProvider{}, c.TestName())
	c.Assert(err, NotNil)
	c.Assert(loader, IsNil)
}
