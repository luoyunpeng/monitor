package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luoyunpeng/monitor/internal/models"
	"github.com/luoyunpeng/monitor/internal/monitor"
)

// Start start the monitor api server
func Start(port string) {
	app := gin.Default()
	app.Use(cors)

	registerRoutes(app)
	srv := &http.Server{
		Addr:    port,
		Handler: app,
	}

	go func() {
		// service connections
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()
	signalListen(srv)
}

func cors(c *gin.Context) {
	whiteList := map[string]int{
		"http://192.168.100.173":      1,
		"http://www.repchain.net.cn":  2,
		"http://localhost:8080":       3,
		"http://test.repchain.net.cn": 4,
		"http://baas.repchain.net.cn": 5,
	}

	// request header
	origin := c.Request.Header.Get("Origin")
	if _, ok := whiteList[origin]; ok {
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		// allow to access the origin
		c.Header("Access-Control-Allow-Origin", origin)
		//all method that server supports, in case of to many pre-checking
		c.Header("Access-Control-Allow-Methods", "POST, GET, PUT, DELETE")
		//  header type
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Length, X-CSRF-Token, Token,session,X_Requested_With,Accept, Origin, Host, Connection, Accept-Encoding, Accept-Language,DNT, X-CustomHeader, Keep-Alive, User-Agent, X-Requested-With, If-Modified-Since, Cache-Control, Content-Type, Pragma")
		// allow across origin setting return other sub fields
		c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers,Cache-Control,Content-Language,Content-Type,Expires,Last-Modified,Pragma,FooBar")
		c.Header("Access-Control-Max-Age", "172800")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Set("content-type", "application/json")
	} else if !ok && origin != "" {
		log.Println("forbid access from origin: ", origin)
	}

	// handle request
	c.Next()
}

func signalListen(srv *http.Server) {
	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-quit
	log.Println("**** Graceful shutdown monitor server ****")

	// release all
	models.StopAllDockerHost()
	if dbCloseErr := monitor.CloseDB(); dbCloseErr != nil {
		log.Printf("Close DB err: %v", dbCloseErr)
	}
	log.Println("close DB done")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Monitor Server shutdown:", err)
	}
	log.Println("**** Monitor server exiting **** ")
}
