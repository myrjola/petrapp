package broker_test

import (
	"github.com/myrjola/petrapp/internal/broker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
)

func TestChannelBroker(t *testing.T) {
	type testCase struct {
		name     string
		testFunc func(b *broker.ChannelBroker[int, string])
	}
	tests := []testCase{
		{
			name: "subscriber receives content",
			testFunc: func(b *broker.ChannelBroker[int, string]) {
				id := 1
				channel := make(chan string)
				b.Publish(id, channel)
				go func() {
					channel <- "hello"
					close(channel)
					b.Unpublish(id)
				}()
				subscriptionChan := <-b.Subscribe(id)
				require.Equal(t, "hello", <-subscriptionChan, "subscriber did not receive content")
				msg, ok := <-subscriptionChan
				require.Empty(t, msg, "subscriber received content after producer closed")
				require.Falsef(t, ok, "channel not closed")
			},
		},
		{
			name: "subsequent subscribers block until producer is finished or unpublished",
			testFunc: func(b *broker.ChannelBroker[int, string]) {
				id := 1
				channel := make(chan string)
				b.Publish(id, channel)
				producerFinished := atomic.Bool{}

				// First subscriber
				subscriptionChan := <-b.Subscribe(id)

				// Next subscriber
				go func() {
					nextSubscriptionChan, ok := <-b.Subscribe(id)
					assert.Nil(t, nextSubscriptionChan, "subsequent subscriber received content")
					assert.Falsef(t, ok, "channel not closed to signal producer is finished")
					assert.True(t, producerFinished.Load(), "producer not finished before subsequent subscriber unblocked")
				}()

				// Finish producer
				go func() {
					channel <- "hello"
					close(channel)
					producerFinished.Store(true)
					b.Unpublish(id)
				}()
				require.Equal(t, "hello", <-subscriptionChan, "subscriber did not receive content")

				// Last subscriber
				nextSubscriptionChan, ok := <-b.Subscribe(id)
				require.Nil(t, nextSubscriptionChan, "last subscriber received content")
				require.Falsef(t, ok, "last subscriber channel not closed to signal producer is finished")
				require.True(t, producerFinished.Load(), "producer not finished before last subscriber unblocked")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := broker.NewChannelBroker[int, string]()
			go br.Start()
			t.Cleanup(func() {
				br.Stop()
			})
			tt.testFunc(br)
		})
	}
}
