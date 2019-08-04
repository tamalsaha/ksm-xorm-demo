package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"xorm.io/core"

	"github.com/go-xorm/xorm"
	_ "github.com/lib/pq"
	"gocloud.dev/secrets"
	_ "gocloud.dev/secrets/gcpkms"
	"gomodules.xyz/secrets/xkms"
)

func main() {
	if err := xormksm_demo(); err != nil {
		log.Fatalln(err)
	}
}

func xormksm_demo() error {
	sakeyFile := "/home/tamal/Downloads/ackube-3b7339da1e1e.json"
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", sakeyFile)

	driver := "postgres"
	ds := fmt.Sprintf("user=%v password=%v host=%v port=%v dbname=%v sslmode=disable",
		"gitea", "gitea", "127.0.0.1", 5432, "xorm-demo")
	masterKeyURL := fmt.Sprintf("gcpkms://projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s", "ackube", "global", "gitea", "gitea-key")

	u := url.URL{
		Scheme: xkms.Scheme,
	}
	q := u.Query()
	q.Set("driver", driver)
	q.Set("ds", ds)
	q.Set("master_key_url", masterKeyURL)
	// q.Set("table", driver)
	u.RawQuery = q.Encode()

	fmt.Println("url", u.String())

	x, err := xorm.NewEngine(driver, ds)
	if err != nil {
		return err
	}
	x.SetMapper(core.GonicMapper{})
	x.ShowSQL(true)

	err = x.CreateTables(&xkms.SecretKey{})
	if err != nil {
		return err
	}

	err = xkms.Register(u.String(), x)
	if err != nil {
		return err
	}

	ctx := context.Background()
	u2 := "xkms://20190803"
	keeper, err := secrets.OpenKeeper(ctx, u2)
	if err != nil {
		return err
	}
	defer keeper.Close()

	err = encdec(keeper, "my name is tamal")
	if err != nil {
		return err
	}
	err = encdec(keeper, "my name is xorm")
	if err != nil {
		return err
	}

	return nil
}

func encdec(keeper *secrets.Keeper, text string) error {
	ctx := context.Background()
	cipher, err := keeper.Encrypt(ctx, []byte(text))
	if err != nil {
		return fmt.Errorf("failed to encrypt: %v", err)
	}
	pt, err := keeper.Decrypt(ctx, cipher)
	if err != nil {
		return fmt.Errorf("failed to decrypt: %v", err)
	}
	fmt.Println(string(pt))
	return nil
}

func gcpkms_demo() error {
	sakeyFile := "/home/tamal/Downloads/ackube-3b7339da1e1e.json"
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", sakeyFile)

	ctx := context.Background()
	url := fmt.Sprintf("gcpkms://projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s", "ackube", "global", "gitea", "gitea-key")
	keeper, err := secrets.OpenKeeper(ctx, url)
	if err != nil {
		return err
	}
	defer keeper.Close()

	cipher, err := keeper.Encrypt(ctx, []byte("my name is tamal"))
	if err != nil {
		return fmt.Errorf("failed to encrypt: %v", err)
	}

	pt, err := keeper.Decrypt(ctx, cipher)
	if err != nil {
		return fmt.Errorf("failed to decrypt: %v", err)
	}
	fmt.Println(string(pt))
	return nil
}
