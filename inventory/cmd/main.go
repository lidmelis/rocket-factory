package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	inventoryv1 "github.com/microservices-course/shared/pkg/proto/inventory/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func main() {
	// Инициализируем тестовые данные
	parts := initTestData()

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	inventoryv1.RegisterInventoryServiceServer(server, &inventoryServer{
		parts: parts,
		mu:    sync.RWMutex{},
	})

	fmt.Println("🚀 InventoryService запущен на :50051")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

type inventoryServer struct {
	inventoryv1.UnimplementedInventoryServiceServer
	parts map[string]*inventoryv1.Part
	mu    sync.RWMutex
}

func (s *inventoryServer) GetPart(ctx context.Context, req *inventoryv1.GetPartRequest) (*inventoryv1.GetPartResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	part, exists := s.parts[req.Uuid]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "часть с UUID %s не найдена", req.Uuid)
	}
	return &inventoryv1.GetPartResponse{Part: part}, nil
}

func (s *inventoryServer) ListParts(ctx context.Context, req *inventoryv1.ListPartsRequest) (*inventoryv1.ListPartsResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*inventoryv1.Part
	filter := req.GetFilter()

	for _, part := range s.parts {
		if matchesFilter(part, filter) {
			result = append(result, part)
		}
	}
	return &inventoryv1.ListPartsResponse{Parts: result}, nil
}

func matchesFilter(part *inventoryv1.Part, filter *inventoryv1.PartsFilter) bool {
	if filter == nil {
		return true
	}

	// Фильтрация по UUID
	if len(filter.Uuids) > 0 {
		found := false
		for _, uuid := range filter.Uuids {
			if part.Uuid == uuid {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Фильтрация по имени
	if len(filter.Names) > 0 {
		found := false
		for _, name := range filter.Names {
			if part.Name == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Фильтрация по категории
	if len(filter.Categories) > 0 {
		found := false
		for _, category := range filter.Categories {
			if part.Category == category {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Фильтрация по стране производителя
	if len(filter.ManufacturerCountries) > 0 {
		found := false
		for _, country := range filter.ManufacturerCountries {
			if part.Manufacturer.Country == country {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Фильтрация по тегам
	if len(filter.Tags) > 0 {
		found := false
		for _, tag := range filter.Tags {
			for _, partTag := range part.Tags {
				if partTag == tag {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func initTestData() map[string]*inventoryv1.Part {
	parts := make(map[string]*inventoryv1.Part)
	now := timestamppb.New(time.Now())

	// Деталь 1: Двигатель
	parts["engine-001"] = &inventoryv1.Part{
		Uuid:          "engine-001",
		Name:          "Главный двигатель Falcon 9",
		Description:   "Многоразовый ракетный двигатель",
		Price:         2500000.00,
		StockQuantity: 5,
		Category:      inventoryv1.Category_CATEGORY_ENGINE,
		Dimensions: &inventoryv1.Dimensions{
			Length: 320.5,
			Width:  180.2,
			Height: 210.7,
			Weight: 4500.0,
		},
		Manufacturer: &inventoryv1.Manufacturer{
			Name:    "SpaceX",
			Country: "USA",
			Website: "https://www.spacex.com",
		},
		Tags: []string{"двигатель", "многоразовый", "космос"},
		Metadata: map[string]*inventoryv1.Value{
			"материал": {Value: &inventoryv1.Value_StringValue{StringValue: "титан"}},
			"тяга":     {Value: &inventoryv1.Value_DoubleValue{DoubleValue: 845.0}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Деталь 2: Топливный бак
	parts["fuel-001"] = &inventoryv1.Part{
		Uuid:          "fuel-001",
		Name:          "Топливный бак LOX",
		Description:   "Бак для жидкого кислорода",
		Price:         1800000.00,
		StockQuantity: 8,
		Category:      inventoryv1.Category_CATEGORY_FUEL,
		Dimensions: &inventoryv1.Dimensions{
			Length: 850.0,
			Width:  420.0,
			Height: 420.0,
			Weight: 3200.0,
		},
		Manufacturer: &inventoryv1.Manufacturer{
			Name:    "Roscosmos",
			Country: "Russia",
			Website: "https://www.roscosmos.ru",
		},
		Tags: []string{"топливо", "бак", "кислород"},
		Metadata: map[string]*inventoryv1.Value{
			"емкость":  {Value: &inventoryv1.Value_DoubleValue{DoubleValue: 287.0}},
			"давление": {Value: &inventoryv1.Value_Int64Value{Int64Value: 350}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Деталь 3: Иллюминатор
	parts["porthole-001"] = &inventoryv1.Part{
		Uuid:          "porthole-001",
		Name:          "Иллюминатор станции",
		Description:   "Кварцевый иллюминатор для МКС",
		Price:         950000.00,
		StockQuantity: 3,
		Category:      inventoryv1.Category_CATEGORY_PORTHOLE,
		Dimensions: &inventoryv1.Dimensions{
			Length: 120.0,
			Width:  120.0,
			Height: 25.0,
			Weight: 180.5,
		},
		Manufacturer: &inventoryv1.Manufacturer{
			Name:    "Boeing",
			Country: "USA",
			Website: "https://www.boeing.com",
		},
		Tags: []string{"иллюминатор", "кварц", "обзор"},
		Metadata: map[string]*inventoryv1.Value{
			"толщина":        {Value: &inventoryv1.Value_DoubleValue{DoubleValue: 12.5}},
			"уровень_защиты": {Value: &inventoryv1.Value_StringValue{StringValue: "IP68"}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Деталь 4: Крыло шаттла
	parts["wing-001"] = &inventoryv1.Part{
		Uuid:          "wing-001",
		Name:          "Крыло космического шаттла",
		Description:   "Теплозащищенное крыло для повторного входа",
		Price:         4200000.00,
		StockQuantity: 2,
		Category:      inventoryv1.Category_CATEGORY_WING,
		Dimensions: &inventoryv1.Dimensions{
			Length: 1850.0,
			Width:  750.0,
			Height: 350.0,
			Weight: 12500.0,
		},
		Manufacturer: &inventoryv1.Manufacturer{
			Name:    "Airbus Defence",
			Country: "Germany",
			Website: "https://www.airbus.com",
		},
		Tags: []string{"крыло", "теплозащита", "шаттл"},
		Metadata: map[string]*inventoryv1.Value{
			"материал":         {Value: &inventoryv1.Value_StringValue{StringValue: "углепластик"}},
			"макс_температура": {Value: &inventoryv1.Value_DoubleValue{DoubleValue: 1650.0}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	return parts
}
