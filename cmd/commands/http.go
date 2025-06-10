package commands

import (
	"context"
	"fmt"
	"github.com/sevlyar/go-daemon"
	"github.com/spf13/cobra"
	"github.com/zapj/zaproxy/http_proxy"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"
)

func init() {
	httpCmd.PersistentFlags().IntP("port", "P", 12828, "Proxy Server Port")
	httpCmd.PersistentFlags().StringP("username", "u", "zaproxy", "username")
	httpCmd.PersistentFlags().StringP("password", "p", "zaproxy", "password")
	rootCmd.AddCommand(httpCmd)
}

var httpCmd = &cobra.Command{
	Use:   "http",
	Short: "start http proxy, default port : 12828",
	Run: func(cmd *cobra.Command, args []string) {
		runDaemon, err := cmd.Flags().GetBool("daemon")
		if err != nil {
			return
		}
		if runDaemon {
			ctx := new(daemon.Context)

			d, err := ctx.Reborn()
			if err != nil {
				log.Fatal("Unable to run: ", err)
			}
			if d != nil {
				return
			}
			defer func(ctx *daemon.Context) {
				err := ctx.Release()
				if err != nil {
					log.Println(err)
				}
			}(ctx)
		}

		startServer(cmd)
	},
}

func startServer(cmd *cobra.Command) {
	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		port = 12828
	}
	username, err := cmd.Flags().GetString("username")
	if err != nil {
		username = "zaproxy"
	}
	password, err := cmd.Flags().GetString("password")
	if err != nil {
		password = "zaproxy"
	}
	proxyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lname, lpass, ok := http_proxy.GetBasicAuth(r)
		//log.Println(username, password)
		if ok {
			if username != lname && password != lpass {
				w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			}
		} else {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}
		path, err := url.Parse("http://" + r.Host)
		if err != nil {
			panic(err)
			return
		}
		proxy := http_proxy.NewReverseProxy(path)
		proxy.ServeHTTP(w, r)
	})

	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: proxyHandler}
	log.Printf("server start : %s", server.Addr)
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	// Waiting for SIGINT (kill -2)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
	log.Println("server exit")
}
