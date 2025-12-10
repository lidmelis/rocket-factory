package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	order_v1 "github.com/microservices-course/shared/pkg/openapi/order/v1"
	inventoryv1 "github.com/microservices-course/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/microservices-course/shared/pkg/proto/payment/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Подключение к InventoryService
	inventoryConn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Не удалось подключиться к InventoryService: %v", err)
	}
	defer inventoryConn.Close()

	inventoryClient := inventoryv1.NewInventoryServiceClient(inventoryConn)

	// Подключение к PaymentService
	paymentConn, err := grpc.NewClient("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Не удалось подключиться к PaymentService: %v", err)
	}
	defer paymentConn.Close()

	paymentClient := paymentv1.NewPaymentServiceClient(paymentConn)

	// Создание OrderService
	orderService := &orderService{
		orders:          make(map[string]*Order),
		mu:              sync.RWMutex{},
		inventoryClient: inventoryClient,
		paymentClient:   paymentClient,
	}

	handler, err := order_v1.NewServer(orderService)
	if err != nil {
		log.Fatalf("Не удалось создать сервер: %v", err)
	}

	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	fmt.Println("🛒 OrderService запущен на :8080")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

type Order struct {
	OrderUUID       string
	UserUUID        uuid.UUID
	PartUUIDs       []uuid.UUID
	TotalPrice      float64
	TransactionUUID *uuid.UUID
	PaymentMethod   order_v1.OptPaymentMethod
	Status          order_v1.OrderStatus
}

type orderService struct {
	order_v1.UnimplementedHandler
	orders          map[string]*Order
	mu              sync.RWMutex
	inventoryClient inventoryv1.InventoryServiceClient
	paymentClient   paymentv1.PaymentServiceClient
}

// Создание заказа
func (s *orderService) CreateOrder(ctx context.Context, req *order_v1.CreateOrderRequest) (order_v1.CreateOrderRes, error) {
	// Конвертируем UUID в строки для запроса к InventoryService
	var partUUIDs []string
	for _, partUUID := range req.PartUuids {
		partUUIDs = append(partUUIDs, partUUID.String())
	}

	// 1. Получаем детали из InventoryService
	partsResp, err := s.inventoryClient.ListParts(ctx, &inventoryv1.ListPartsRequest{
		Filter: &inventoryv1.PartsFilter{
			Uuids: partUUIDs,
		},
	})
	if err != nil {
		return &order_v1.InternalServerError{
			Message: fmt.Sprintf("Ошибка при получении деталей: %v", err),
		}, nil
	}

	// 2. Проверяем, что все детали существуют
	if len(partsResp.Parts) != len(req.PartUuids) {
		return &order_v1.BadRequestError{
			Message: "Некоторые детали не найдены",
		}, nil
	}

	// 3. Рассчитываем общую стоимость
	var totalPrice float64
	for _, part := range partsResp.Parts {
		totalPrice += part.Price
	}

	// 4. Создаем заказ
	orderUUID := uuid.New()

	order := &Order{
		OrderUUID:  orderUUID.String(),
		UserUUID:   req.UserUUID,
		PartUUIDs:  req.PartUuids,
		TotalPrice: totalPrice,
		Status:     order_v1.OrderStatusPENDINGPAYMENT,
		PaymentMethod: order_v1.OptPaymentMethod{
			Set: false,
		},
	}

	s.mu.Lock()
	s.orders[orderUUID.String()] = order
	s.mu.Unlock()

	fmt.Printf("📝 Создан заказ: %s, пользователь: %s, сумма: %.2f\n",
		orderUUID, req.UserUUID, totalPrice)

	return &order_v1.CreateOrderResponse{
		OrderUUID:  orderUUID,
		TotalPrice: totalPrice,
	}, nil
}

// Получение заказа по UUID
func (s *orderService) GetOrder(ctx context.Context, params order_v1.GetOrderParams) (order_v1.GetOrderRes, error) {
	orderUUID := params.OrderUUID.String()

	s.mu.RLock()
	order, exists := s.orders[orderUUID]
	s.mu.RUnlock()

	if !exists {
		return &order_v1.NotFoundError{
			Message: "Заказ не найден",
		}, nil
	}

	var transactionUUID order_v1.OptNilUUID
	if order.TransactionUUID != nil {
		transactionUUID = order_v1.NewOptNilUUID(*order.TransactionUUID)
	}

	return &order_v1.OrderDto{
		OrderUUID:       uuid.MustParse(order.OrderUUID),
		UserUUID:        order.UserUUID,
		PartUuids:       order.PartUUIDs,
		TotalPrice:      order.TotalPrice,
		TransactionUUID: transactionUUID,
		PaymentMethod:   order.PaymentMethod,
		Status:          order.Status,
	}, nil
}

// Оплата заказа
func (s *orderService) PayOrder(ctx context.Context, req *order_v1.PayOrderRequest, params order_v1.PayOrderParams) (order_v1.PayOrderRes, error) {
	orderUUID := params.OrderUUID.String()

	s.mu.Lock()
	order, exists := s.orders[orderUUID]
	s.mu.Unlock()

	if !exists {
		return &order_v1.NotFoundError{
			Message: "Заказ не найден",
		}, nil
	}

	if order.Status != order_v1.OrderStatusPENDINGPAYMENT {
		return &order_v1.InternalServerError{
			Message: "Заказ уже оплачен или отменен",
		}, nil
	}

	// Конвертируем PaymentMethod для gRPC
	var paymentMethod paymentv1.PaymentMethod
	switch req.PaymentMethod {
	case order_v1.PaymentMethodCARD:
		paymentMethod = paymentv1.PaymentMethod_PAYMENT_METHOD_CARD
	case order_v1.PaymentMethodSBP:
		paymentMethod = paymentv1.PaymentMethod_PAYMENT_METHOD_SBP
	case order_v1.PaymentMethodCREDITCARD:
		paymentMethod = paymentv1.PaymentMethod_PAYMENT_METHOD_CREDIT_CARD
	case order_v1.PaymentMethodINVESTORMONEY:
		paymentMethod = paymentv1.PaymentMethod_PAYMENT_METHOD_INVESTOR_MONEY
	default:
		paymentMethod = paymentv1.PaymentMethod_PAYMENT_METHOD_UNSPECIFIED
	}

	// Вызываем PaymentService
	paymentResp, err := s.paymentClient.PayOrder(ctx, &paymentv1.PayOrderRequest{
		OrderUuid:     orderUUID,
		UserUuid:      order.UserUUID.String(),
		PaymentMethod: paymentMethod,
	})
	if err != nil {
		return &order_v1.InternalServerError{
			Message: fmt.Sprintf("Ошибка при оплате: %v", err),
		}, nil
	}

	// Обновляем заказ
	transactionUUID, err := uuid.Parse(paymentResp.TransactionUuid)
	if err != nil {
		return &order_v1.InternalServerError{
			Message: "Ошибка при обработке UUID транзакции",
		}, nil
	}

	order.Status = order_v1.OrderStatusPAID
	order.TransactionUUID = &transactionUUID
	order.PaymentMethod = order_v1.OptPaymentMethod{
		Value: req.PaymentMethod,
		Set:   true,
	}

	s.mu.Lock()
	s.orders[orderUUID] = order
	s.mu.Unlock()

	fmt.Printf("💰 Оплачен заказ: %s, транзакция: %s\n",
		orderUUID, transactionUUID)

	return &order_v1.PayOrderResponse{
		TransactionUUID: transactionUUID,
	}, nil
}

// Отмена заказа
func (s *orderService) CancelOrder(ctx context.Context, params order_v1.CancelOrderParams) (order_v1.CancelOrderRes, error) {
	orderUUID := params.OrderUUID.String()

	s.mu.Lock()
	defer s.mu.Unlock()

	order, exists := s.orders[orderUUID]
	if !exists {
		return &order_v1.NotFoundError{
			Message: "Заказ не найден",
		}, nil
	}

	// Проверяем, можно ли отменить заказ
	if order.Status == order_v1.OrderStatusPAID {
		return &order_v1.ConflictError{
			Message: "Нельзя отменить оплаченный заказ",
		}, nil
	}

	if order.Status == order_v1.OrderStatusCANCELLED {
		return &order_v1.CancelOrderNoContent{}, nil
	}

	// Отменяем заказ
	order.Status = order_v1.OrderStatusCANCELLED
	s.orders[orderUUID] = order

	fmt.Printf("❌ Отменен заказ: %s\n", orderUUID)

	return &order_v1.CancelOrderNoContent{}, nil
}
