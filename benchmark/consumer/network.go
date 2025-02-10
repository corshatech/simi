/*
Copyright Corsha Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package consumer

import (
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	benchmarksQueueName = "benchmarks"
	regQueueName        = "registrations"

	protobufContentType = "application/x-protobuf"
	jsonContentType     = "application/json"
	regContentType      = "text/plain"
)

// SyncRegistrations waits until the expected number of registrations have been received.
// It then broadcasts a Start message using an AMQP topic.
func SyncRegistrations(conn *amqp.Connection, expectedRegistrations uint32) (uint32, error) {
	ch, err := conn.Channel()
	if err != nil {
		return 0, fmt.Errorf("failed to create AMQP channel: %w", err)
	}
	defer ch.Close()

	_, err = ch.QueueDeclare(
		regQueueName, // name
		false,        // durable
		false,        // delete when unused
		false,        // exclusive
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return 0, err
	}

	regDelivery, err := ch.Consume(
		regQueueName, // queue name
		"",           // consumer name
		true,         // autoAck
		true,         // exclusive
		false,        // noLocal (unsupported)
		false,        // noWait
		nil)          // args
	if err != nil {
		return 0, err
	}

	numRegistered := waitForRegistrations(regDelivery, expectedRegistrations)

	// All registrations received, broadcast "Start" message
	err = ch.Publish(
		"amq.topic", // topic exchange
		"start",     // routing key
		true,        // mandatory
		false,       // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte("start"),
		})
	if err != nil {
		return 0, fmt.Errorf("failed to send start command to workers: %w", err)
	}

	log.Printf("Broadcasted Start command to %d workers\n", numRegistered)

	return numRegistered, nil
}

func waitForRegistrations(regDelivery <-chan amqp.Delivery, expectedRegistrations uint32) uint32 {
	log.Printf("Waiting for %d registrations", expectedRegistrations)

	// Handle registration results
	var errCount, regCount uint32
	for regStatus := range regDelivery {
		if regStatus.ContentType != regContentType {
			log.Printf("Ignoring result with unexpected content-type: %s", regStatus.ContentType)
			continue
		}

		regCount++

		if len(regStatus.Body) > 0 {
			log.Printf("Failed registration from %s: %s", regStatus.CorrelationId, string(regStatus.Body))
			errCount++
		} else {
			log.Printf("Successful registration from %s (%d/%d)", regStatus.CorrelationId, regCount, expectedRegistrations)
		}

		if regCount == expectedRegistrations {
			break
		}
	}

	if errCount == expectedRegistrations {
		log.Fatalln("All registrations failed, exiting")
	}

	log.Printf("Registration completed with %d errors", errCount)
	return regCount - errCount
}
