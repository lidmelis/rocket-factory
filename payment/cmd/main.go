package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/google/uuid"
	paymentv1 "github.com/microservices-course/shared/pkg/proto/payment/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	paymentv1.RegisterPaymentServiceServer(server, &paymentServer{})

	reflection.Register(server)
	fmt.Println("💰 PaymentService запущен на :50052")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

type paymentServer struct {
	paymentv1.UnimplementedPaymentServiceServer
}

func (s *paymentServer) PayOrder(ctx context.Context, req *paymentv1.PayOrderRequest) (*paymentv1.PayOrderResponse, error) {
	// Симуляция обработки платежа
	time.Sleep(100 * time.Millisecond)

	transactionUUID := uuid.New().String()

	fmt.Printf("💳 Оплата прошла успешно, transaction_uuid: %s\n", transactionUUID)
	fmt.Printf("   Order: %s, User: %s, Method: %v\n",
		req.OrderUuid, req.UserUuid, req.PaymentMethod)

	return &paymentv1.PayOrderResponse{
		TransactionUuid: transactionUUID,
	}, nil
}
