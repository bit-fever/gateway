//=============================================================================
/*
Copyright © 2023 Andrea Carboni andrea.carboni71@gmail.com

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
//=============================================================================

package service

import (
	"crypto/tls"
	"crypto/x509"
	"github.com/bit-fever/core"
	"github.com/bit-fever/gateway/pkg/app"
	"github.com/gin-gonic/gin"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

//=============================================================================

var gatewayCfg *app.Config
var transportCfg *http.Transport

//=============================================================================

func Init(cfg *app.Config, router *gin.Engine, logger *slog.Logger) {
	gatewayCfg   = cfg
	transportCfg = createHttpTransport(logger)
	router.Use(handleUrl)
}

//=============================================================================

func createHttpTransport(logger *slog.Logger) *http.Transport {
	cert, err := os.ReadFile("config/ca.crt")
	if err != nil {
		core.ExitWithMessage("Could not open certificate file: "+ err.Error())
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(cert)

	certificate, err := tls.LoadX509KeyPair("config/client.crt", "config/client.key")
	if err != nil {
		core.ExitWithMessage("Could not load certificate: "+ err.Error())
	}

	return &http.Transport{
		TLSClientConfig: &tls.Config{
				RootCAs:      caCertPool,
				Certificates: []tls.Certificate{certificate},
			},
		}
}

//=============================================================================

func handleUrl(c *gin.Context) {
	start := time.Now()
	slog.Info("New request", "client", c.ClientIP(), "context", c.Request.URL.String())
	path := c.Request.URL.Path

	targetURL := lookupTargetURL(path)
	if targetURL == "" {
		c.String(404, "Not Found")
		slog.Error("URL mapping not found", "client", c.ClientIP(), "path", path)
		return
	}

	proxy(targetURL, c)
	duration := time.Since(start)
	slog.Info("Request served", "duration", duration.Seconds())
}

//=============================================================================

func lookupTargetURL(path string) string {
	prefix := ""
	target := ""

	for _, elem := range gatewayCfg.Routes {
		subPath, found := strings.CutPrefix(path, elem.Prefix)
		if found && len(prefix) < len(elem.Prefix) {
			if !strings.HasPrefix(subPath, "/") {
				subPath = "/" + subPath
			}

			prefix = elem.Prefix
			target = elem.Url + subPath
		}
	}

	return target
}

//=============================================================================

func proxy(targetURL string, c *gin.Context) {
	target, err := url.Parse(targetURL)
	if err != nil {
		c.String(500, "Invalid target URL")
		slog.Error("Invalid target URL", "client", c.ClientIP(), "url", targetURL)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transportCfg
	proxy.Director  = func(request *http.Request) {
		request.URL.Scheme = target.Scheme
		request.URL.Host = target.Host
		request.URL.Path = target.Path
	}

	slog.Info("Forwarding request", "target", target)
	proxy.ServeHTTP(c.Writer, c.Request)
}

//=============================================================================
