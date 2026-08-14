// Package mqttclient is the concrete MQTT client behind provider.Subscriber.
//
// Kept out of internal/provider on purpose: the provider declares the slice of
// a client it needs and stays dependency-free, and this package brings paho —
// the dependency, in one place, chosen because it is the reference Go client
// and the alternative was writing reconnection logic ourselves.
package mqttclient

import (
	"context"
	"fmt"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/MeshBench/meshbench/internal/provider"
)

// Client subscribes over a real broker connection.
type Client struct {
	// BrokerURL is what paho expects: tcp://host:1883, ssl://host:8883,
	// ws://host/path.
	BrokerURL string
	Username  string
	Password  string
}

var _ provider.Subscriber = (*Client)(nil)

// Subscribe connects, subscribes, and delivers until the context ends.
//
// The connection lives exactly as long as the subscription: a live feed with
// no subscriber has no reason to hold a broker connection open.
func (c *Client) Subscribe(ctx context.Context, topic string, fn func(provider.Message)) error {
	opts := paho.NewClientOptions().
		AddBroker(c.BrokerURL).
		SetClientID(fmt.Sprintf("meshcoresim-%d", time.Now().UnixNano())).
		SetConnectTimeout(10 * time.Second).
		SetAutoReconnect(true)
	if c.Username != "" {
		opts.SetUsername(c.Username)
		opts.SetPassword(c.Password)
	}
	cl := paho.NewClient(opts)
	if tok := cl.Connect(); tok.Wait() && tok.Error() != nil {
		return fmt.Errorf("mqtt: connect %s: %w", c.BrokerURL, tok.Error())
	}
	defer cl.Disconnect(250)

	if tok := cl.Subscribe(topic, 0, func(_ paho.Client, m paho.Message) {
		fn(provider.Message{Topic: m.Topic(), Payload: m.Payload()})
	}); tok.Wait() && tok.Error() != nil {
		return fmt.Errorf("mqtt: subscribe %q: %w", topic, tok.Error())
	}
	<-ctx.Done()
	return ctx.Err()
}
