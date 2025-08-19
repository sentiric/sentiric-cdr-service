// ========== FILE: sentiric-cdr-service/internal/client/grpc_client.go ==========
package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sentiric/sentiric-cdr-service/internal/config"
	userv1 "github.com/sentiric/sentiric-contracts/gen/go/sentiric/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func NewUserServiceClient(cfg *config.Config) (userv1.UserServiceClient, error) {
	conn, err := createGrpcClient(cfg, cfg.UserServiceGrpcURL)
	if err != nil {
		return nil, fmt.Errorf("user service istemcisi için bağlantı oluşturulamadı: %w", err)
	}
	return userv1.NewUserServiceClient(conn), nil
}

func createGrpcClient(cfg *config.Config, addr string) (*grpc.ClientConn, error) {
	clientCert, err := tls.LoadX509KeyPair(cfg.CdrServiceCertPath, cfg.CdrServiceKeyPath)
	if err != nil {
		return nil, fmt.Errorf("istemci sertifikası yüklenemedi: %w", err)
	}

	caCert, err := os.ReadFile(cfg.GrpcTlsCaPath)
	if err != nil {
		return nil, fmt.Errorf("CA sertifikası okunamadı: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("CA sertifikası havuza eklenemedi")
	}

	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
		ServerName:   strings.Split(addr, ":")[0],
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// grpc.NewClient yerine grpc.DialContext kullanarak context'i kullanıyoruz.
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("gRPC sunucusuna (%s) bağlanılamadı: %w", addr, err)
	}

	return conn, nil
}
