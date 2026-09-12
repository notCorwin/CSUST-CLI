package adapter

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMailLoginUsesProviderRSAContract(t *testing.T) {
	privateKey, err := rsa.GenerateKey(cryptorand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	var decrypted string
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(writer, `<html><form id="loginform"><select name="domain"><option value="csust.edu.cn">csust.edu.cn</option></select></form><script>var entryHost = %q; var entrybjhost = %q;</script><a attr-ch="重置密码" href="/reset"></a>`, serverURL, serverURL)
		case "/login/prelogin.jsp":
			callback := request.URL.Query().Get("callback")
			writer.Header().Set("Content-Type", "application/javascript")
			_, _ = fmt.Fprintf(writer, `%s({"code":200,"data":{"rand":"nonce","pubid":"pub-1","modulus":"%x","exponent":"010001"}})`, callback, privateKey.PublicKey.N)
		case "/login/domainEntLogin":
			if err := request.ParseForm(); err != nil {
				t.Error(err)
			}
			if request.Form.Get("account_name") != "alice" || request.Form.Get("domain") != "csust.edu.cn" || request.Form.Get("pubid") != "pub-1" || request.Form.Get("passtype") != "3" {
				t.Errorf("unexpected login fields: %#v", request.Form)
			}
			ciphertext, decodeErr := hexDecode(request.Form.Get("password"))
			if decodeErr != nil {
				t.Error(decodeErr)
			} else if plain, decryptErr := rsa.DecryptPKCS1v15(cryptorand.Reader, privateKey, ciphertext); decryptErr != nil {
				t.Error(decryptErr)
			} else {
				decrypted = string(plain)
			}
			writer.Header().Set("Set-Cookie", "mail_session=ok; Path=/")
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = writer.Write([]byte(`<html><body>邮箱首页</body></html>`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	t.Setenv("CSUST_BASE_URL", server.URL)
	cookie := filepath.Join(t.TempDir(), "cookies.txt")
	result := runIssueJSON(t, "mail", "login", "--username", "alice@csust.edu.cn", "--password", "secret", "--cookie-file", cookie)
	if result["confirmed"] != true || result["submitted"] != true || result["username"] != "alice" {
		t.Fatalf("mail login was not confirmed: %#v", result)
	}
	if decrypted != "secret#nonce" {
		t.Fatalf("provider RSA payload was not decrypted as expected: %q", decrypted)
	}
	content, readErr := os.ReadFile(cookie)
	if readErr != nil || !strings.Contains(string(content), "mail_session") {
		t.Fatalf("mail session cookie was not saved: %v", readErr)
	}
}

func hexDecode(value string) ([]byte, error) {
	return hex.DecodeString(value)
}
