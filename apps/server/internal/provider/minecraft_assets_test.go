package provider

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"testing"
)

func TestMinecraftAssetTrustAndChecksum(t *testing.T) {
	for _, s := range []string{"http://piston-data.mojang.com/a", "https://localhost/a", "https://piston-data.mojang.com:444/a", "https://user:password@piston-data.mojang.com/a", "https://piston-data.mojang.com.evil.invalid/a"} {
		u, _ := url.Parse(s)
		if minecraftAssetURL(u) {
			t.Errorf("unsafe URL allowed %s", s)
		}
	}
	u, _ := url.Parse("https://piston-data.mojang.com/v1/objects/abc/client.jar")
	if !minecraftAssetURL(u) {
		t.Fatal("official URL rejected")
	}
	if !minecraftSHA([]byte("abc"), "a9993e364706816aba3e25717850c26c9cd0d89d") || minecraftSHA([]byte("altered"), "a9993e364706816aba3e25717850c26c9cd0d89d") {
		t.Fatal("checksum validation broken")
	}
}

func TestFetchMinecraftLanguageLive(t *testing.T) {
	if os.Getenv("MPACKSTATION_TEST_MOJANG") == "" {
		t.Skip("set MPACKSTATION_TEST_MOJANG for live metadata verification")
	}
	raw, err := FetchMinecraftLanguage(context.Background(), "1.21.1", "zh_cn")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if json.Unmarshal(raw, &values) != nil || values["item.minecraft.iron_ingot"] != "铁锭" || values["item.minecraft.redstone"] != "红石粉" || values["block.minecraft.cobblestone"] != "圆石" || values["item.minecraft.diamond"] != "钻石" {
		t.Fatalf("unexpected zh_cn sample values: %#v", values)
	}
}
