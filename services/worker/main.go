package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var sqsClient *SQSClient

func main() {
	if os.Getenv("SQS_QUEUE_URL") == "" {
		log.Fatal("SQS_QUEUE_URL is required")
	}

	var err error
	sqsClient, err = newSQSClient()
	if err != nil {
		log.Fatalf("failed to init SQS client: %v", err)
	}

	services := map[string]string{
		"inventory":    getEnv("INVENTORY_SERVICE_URL", "http://inventory-service:8082"),
		"payment":      getEnv("PAYMENT_SERVICE_URL", "http://payment-service:8083"),
		"notification": getEnv("NOTIFICATION_SERVICE_URL", "http://notification-service:8084"),
		"shipping":     getEnv("SHIPPING_SERVICE_URL", "http://shipping-service:8085"),
		"order":        getEnv("ORDER_SERVICE_URL", "http://order-service:8081"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "worker"})
		})
		port := getEnv("HEALTH_PORT", "8090")
		log.Printf("Worker health check on :%s", port)
		http.ListenAndServe(":"+port, mux)
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down worker...")
		cancel()
	}()

	log.Println("Worker started, polling SQS for events...")
	pollAndProcess(ctx, services)
}

func pollAndProcess(ctx context.Context, services map[string]string) {
	client := &http.Client{Timeout: 10 * time.Second}

	for {
		select {
		case <-ctx.Done():
			log.Println("Worker stopped")
			return
		default:
			messages, handles, err := sqsClient.receive(ctx)
			if err != nil {
				if ctx.Err() != nil {
					continue
				}
				log.Printf("ERROR: receive failed: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			for i, event := range messages {
				log.Printf("Processing event: %s", event.Type)

				if err := handleEvent(client, services, event); err != nil {
					log.Printf("Failed to handle event %s: %v (will retry via SQS)", event.Type, err)
					continue
				}

				if err := sqsClient.delete(ctx, handles[i]); err != nil {
					log.Printf("WARNING: failed to delete message: %v", err)
				}
				log.Printf("Successfully processed: %s", event.Type)
			}
		}
	}
}

func handleEvent(client *http.Client, services map[string]string, event SQSMessage) error {
	switch event.Type {

	case "order.created":
		orderID := sqsGetInt(event.Payload, "order_id")
		customerID := sqsGetString(event.Payload, "customer_id")
		total := sqsGetFloat(event.Payload, "total")
		currency := sqsGetString(event.Payload, "currency")

		log.Printf("  -> Reserving inventory for order #%d", orderID)
		code, err := postJSON(client, services["inventory"]+"/reserve", map[string]interface{}{
			"order_id": orderID,
			"items":    event.Payload["items"],
		})
		if err != nil {
			return fmt.Errorf("inventory unreachable: %w", err)
		}
		if code != http.StatusCreated {
			log.Printf("  -> Reservation failed (%d), cancelling order #%d", code, orderID)
			putJSON(client, services["order"]+"/status", map[string]interface{}{
				"order_id": orderID, "new_status": "cancelled",
			})
			postJSON(client, services["notification"]+"/send", map[string]interface{}{
				"recipient": customerID, "template": "payment_failed",
				"data": map[string]interface{}{"OrderID": orderID},
			})
			return nil
		}

		log.Printf("  -> Processing payment for order #%d", orderID)
		code, err = postJSON(client, services["payment"]+"/charge", map[string]interface{}{
			"order_id":    orderID,
			"customer_id": customerID,
			"amount":      total,
			"currency":    currency,
			"method":      "card",
		})
		if err != nil {
			postJSON(client, services["inventory"]+"/release", map[string]interface{}{"order_id": orderID})
			return fmt.Errorf("payment unreachable: %w", err)
		}
		if code == http.StatusPaymentRequired {
			log.Printf("  -> Payment declined for order #%d, cancelling", orderID)
			postJSON(client, services["inventory"]+"/release", map[string]interface{}{"order_id": orderID})
			putJSON(client, services["order"]+"/status", map[string]interface{}{
				"order_id": orderID, "new_status": "cancelled",
			})
			postJSON(client, services["notification"]+"/send", map[string]interface{}{
				"recipient": customerID, "template": "payment_failed",
				"data": map[string]interface{}{"OrderID": orderID},
			})
			return nil
		}
		if code != http.StatusCreated {
			return fmt.Errorf("unexpected charge response: %d", code)
		}

		log.Printf("  -> Sending order confirmation")
		postJSON(client, services["notification"]+"/send", map[string]interface{}{
			"recipient": customerID, "template": "order_confirmed",
			"data": map[string]interface{}{"OrderID": orderID, "Total": total, "Currency": currency},
		})

		log.Printf("  -> Confirming order #%d", orderID)
		code, err = putJSON(client, services["order"]+"/status", map[string]interface{}{
			"order_id": orderID, "new_status": "confirmed",
		})
		if err != nil {
			return fmt.Errorf("order service unreachable: %w", err)
		}
		if code != http.StatusOK && code != http.StatusConflict {
			return fmt.Errorf("unexpected status update response: %d", code)
		}

	case "order.status_changed":
		orderID := sqsGetInt(event.Payload, "order_id")
		newStatus := sqsGetString(event.Payload, "new_status")

		switch newStatus {
		case "confirmed":
			log.Printf("  -> Order #%d confirmed, moving to processing", orderID)
			code, err := putJSON(client, services["order"]+"/status", map[string]interface{}{
				"order_id": orderID, "new_status": "processing",
			})
			if err != nil {
				return fmt.Errorf("order service unreachable: %w", err)
			}
			if code != http.StatusOK && code != http.StatusConflict {
				return fmt.Errorf("unexpected response: %d", code)
			}

		case "processing":
			log.Printf("  -> Creating shipment for order #%d", orderID)
			code, err := postJSON(client, services["shipping"]+"/shipments", map[string]interface{}{
				"order_id":       orderID,
				"carrier":        "royal_mail",
				"recipient_name": "Customer",
				"address_line1":  "1 Demo Street",
				"city":           "London",
				"postcode":       "E1 1AA",
				"country":        "GB",
				"weight_kg":      1.0,
			})
			if err != nil {
				return fmt.Errorf("shipping unreachable: %w", err)
			}
			if code != http.StatusCreated {
				return fmt.Errorf("shipment creation failed: %d", code)
			}

		case "shipped":
			log.Printf("  -> Sending shipping notification")
			postJSON(client, services["notification"]+"/send", map[string]interface{}{
				"recipient": "customer", "template": "order_shipped",
				"data": map[string]interface{}{"OrderID": orderID},
			})

		case "delivered":
			log.Printf("  -> Sending delivery notification")
			postJSON(client, services["notification"]+"/send", map[string]interface{}{
				"recipient": "customer", "template": "order_delivered",
				"data": map[string]interface{}{"OrderID": orderID},
			})

		case "cancelled":
			log.Printf("  -> Releasing inventory for cancelled order #%d", orderID)
			if _, err := postJSON(client, services["inventory"]+"/release", map[string]interface{}{
				"order_id": orderID,
			}); err != nil {
				return fmt.Errorf("inventory unreachable: %w", err)
			}
		}

	case "payment.completed":
		orderID := sqsGetInt(event.Payload, "order_id")
		log.Printf("  -> Payment completed for order #%d, ensuring confirmed", orderID)
		code, err := putJSON(client, services["order"]+"/status", map[string]interface{}{
			"order_id": orderID, "new_status": "confirmed",
		})
		if err != nil {
			return fmt.Errorf("order service unreachable: %w", err)
		}
		if code != http.StatusOK && code != http.StatusConflict {
			return fmt.Errorf("unexpected response: %d", code)
		}

	case "payment.failed":
		orderID := sqsGetInt(event.Payload, "order_id")
		log.Printf("  -> Payment failed for order #%d, ensuring cancelled", orderID)
		postJSON(client, services["inventory"]+"/release", map[string]interface{}{"order_id": orderID})
		code, err := putJSON(client, services["order"]+"/status", map[string]interface{}{
			"order_id": orderID, "new_status": "cancelled",
		})
		if err != nil {
			return fmt.Errorf("order service unreachable: %w", err)
		}
		if code != http.StatusOK && code != http.StatusConflict {
			return fmt.Errorf("unexpected response: %d", code)
		}

	case "payment.refunded":
		log.Printf("  -> Refund processed for order #%d", sqsGetInt(event.Payload, "order_id"))

	case "shipment.created":
		log.Printf("  -> Shipment created for order #%d", sqsGetInt(event.Payload, "order_id"))

	case "shipment.in_transit":
		orderID := sqsGetInt(event.Payload, "order_id")
		log.Printf("  -> Shipment in transit, marking order #%d shipped", orderID)
		code, err := putJSON(client, services["order"]+"/status", map[string]interface{}{
			"order_id": orderID, "new_status": "shipped",
		})
		if err != nil {
			return fmt.Errorf("order service unreachable: %w", err)
		}
		if code != http.StatusOK && code != http.StatusConflict {
			return fmt.Errorf("unexpected response: %d", code)
		}

	case "shipment.delivered":
		orderID := sqsGetInt(event.Payload, "order_id")
		log.Printf("  -> Shipment delivered, marking order #%d delivered", orderID)
		code, err := putJSON(client, services["order"]+"/status", map[string]interface{}{
			"order_id": orderID, "new_status": "delivered",
		})
		if err != nil {
			return fmt.Errorf("order service unreachable: %w", err)
		}
		if code != http.StatusOK && code != http.StatusConflict {
			return fmt.Errorf("unexpected response: %d", code)
		}

	default:
		log.Printf("  -> Unknown event type: %s (skipping)", event.Type)
	}

	return nil
}

func postJSON(client *http.Client, url string, body interface{}) (int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func putJSON(client *http.Client, url string, body interface{}) (int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
