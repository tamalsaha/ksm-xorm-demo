package xkms

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"

	"github.com/go-xorm/xorm"
	"gocloud.dev/secrets"
	"gocloud.dev/secrets/localsecrets"
	"gomodules.xyz/secrets/types"
	"xorm.io/core"
)

func init() {
	secrets.DefaultURLMux().RegisterKeeper(Scheme, &URLOpener{})
}

// Scheme is the URL scheme xoem registers its URLOpener under on
// secrets.DefaultMux.
// See the package documentation and/or URLOpener for details.
const (
	Scheme = "xkms"
)

type connector struct {
	Driver     string
	DataSource string
}

type Options struct {
	conn         connector
	Table        string
	MasterKeyURL string
}

var defaultOptions Options

var m sync.RWMutex
var engines = make(map[connector]*xorm.Engine)

func Init(rawurl string) error {
	u, err := url.Parse(rawurl)
	if err != nil {
		return err
	}
	q := u.Query()

	defaultOptions.conn.Driver = q.Get("driver")
	defaultOptions.conn.DataSource = q.Get("ds")
	defaultOptions.MasterKeyURL = q.Get("master_key_url")
	if (defaultOptions.conn.Driver == "") || (defaultOptions.conn.DataSource == "") || (defaultOptions.MasterKeyURL == "") {
		return errors.New("must supply driver, ds & master_key_url query parameters")
	}

	m.RLock()
	_, ok := engines[defaultOptions.conn]
	m.RUnlock()
	if ok {
		return fmt.Errorf("engine for %#v has been already initialized", defaultOptions.conn)
	}

	defaultOptions.Table = q.Get("table")
	if defaultOptions.Table == "" {
		defaultOptions.Table = "secret_key"
	}

	x, err := xorm.NewEngine(defaultOptions.conn.Driver, defaultOptions.conn.DataSource)
	if err != nil {
		return err
	}
	x.SetCacher(defaultOptions.Table, xorm.NewLRUCacher(xorm.NewMemoryStore(), 50))
	x.SetMapper(core.GonicMapper{})
	x.ShowSQL(true)

	m.Lock()
	defer m.Unlock()
	_, ok = engines[defaultOptions.conn]
	if ok {
		x.Close()
		return fmt.Errorf("engine for %#v has been already initialized", defaultOptions.conn)
	}
	engines[defaultOptions.conn] = x

	return nil
}

func Register(rawurl string, x *xorm.Engine) error {
	u, err := url.Parse(rawurl)
	if err != nil {
		return err
	}
	q := u.Query()

	defaultOptions.conn.Driver = q.Get("driver")
	defaultOptions.conn.DataSource = q.Get("ds")
	defaultOptions.MasterKeyURL = q.Get("master_key_url")
	if (defaultOptions.conn.Driver == "") || (defaultOptions.conn.DataSource == "") || (defaultOptions.MasterKeyURL == "") {
		return errors.New("must supply driver, ds & master_key_url query parameters")
	}

	m.RLock()
	_, ok := engines[defaultOptions.conn]
	m.RUnlock()
	if ok {
		return fmt.Errorf("engine for %#v has been already initialized", defaultOptions.conn)
	}

	defaultOptions.Table = q.Get("table")
	if defaultOptions.Table == "" {
		defaultOptions.Table = "secret_key"
	}

	m.Lock()
	defer m.Unlock()
	_, ok = engines[defaultOptions.conn]
	if ok {
		return fmt.Errorf("engine for %#v has been already initialized", defaultOptions.conn)
	}
	engines[defaultOptions.conn] = x

	return nil
}

// URLOpener opens xorm URLs like "xorm://keyId?driver=postgres&ds=connection_string&table=table_name".
type URLOpener struct{}

// OpenKeeperURL opens Keeper URLs.
func (o *URLOpener) OpenKeeperURL(ctx context.Context, u *url.URL) (*secrets.Keeper, error) {
	id, err := strconv.ParseInt(u.Host, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("could not parse id=%v, err:%v", u.Path, err)
	}

	for k := range u.Query() {
		if k != "driver" && k != "ds" && k != "table" && k != "master_key_url" {
			return nil, fmt.Errorf("invalid query parameter %q", k)
		}
	}
	opts := new(Options)
	*opts = defaultOptions

	if v := u.Query().Get("driver"); v != "" {
		opts.conn.Driver = v
	}
	if v := u.Query().Get("ds"); v != "" {
		opts.conn.DataSource = v
	}
	if v := u.Query().Get("master_key_url"); v != "" {
		opts.MasterKeyURL = v
	}
	if v := u.Query().Get("table"); v != "" {
		opts.Table = v
	}

	if (opts.conn.Driver == "") || (opts.conn.DataSource == "") || (opts.Table == "") || (opts.MasterKeyURL == "") {
		return nil, errors.New("must supply driver, ds, master_key_url and table query parameters")
	}

	m.RLock()
	x, ok := engines[opts.conn]
	m.RUnlock()

	if !ok {
		x2, err := xorm.NewEngine(opts.conn.Driver, opts.conn.DataSource)
		if err != nil {
			return nil, err
		}
		x2.SetCacher(defaultOptions.Table, xorm.NewLRUCacher(xorm.NewMemoryStore(), 50))
		x2.SetMapper(core.GonicMapper{})
		x2.ShowSQL(true)

		m.Lock()
		x, ok = engines[opts.conn]
		if ok {
			x2.Close()
		} else {
			x = x2
			engines[opts.conn] = x
		}
		m.Unlock()
	}

	data := SecretKey{ID: id}
	found, err := x.Table(opts.Table).Get(&data)
	if err != nil {
		return nil, fmt.Errorf("failed to load id=%d, err:%v", id, err)
	}

	var sk [32]byte
	if found {
		sk, err = localsecrets.Base64Key(data.Key.Data)
		if err != nil {
			return nil, fmt.Errorf("open keeper %v: failed to get key: %v", u, err)
		}
	} else {
		sk, err = localsecrets.NewRandomKey()
		if err != nil {
			return nil, fmt.Errorf("open keeper %v: failed to get key: %v", u, err)
		}
		_, err := x.Table(opts.Table).InsertOne(&SecretKey{
			ID: id,
			Key: types.SecureString{
				URL:  opts.MasterKeyURL,
				Data: base64.StdEncoding.EncodeToString(sk[:]),
			},
		})
		if err != nil {
			return nil, fmt.Errorf("open keeper %v: failed to get key: %v", u, err)
		}
	}
	return localsecrets.NewKeeper(sk), nil
}

type SecretKey struct {
	ID  int64              `xorm:"pk autoincr"`
	Key types.SecureString `xorm:"text"`

	CreatedUnix int64 `xorm:"INDEX created"`
}
