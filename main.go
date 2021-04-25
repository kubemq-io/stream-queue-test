package main

import (
	"context"
	"fmt"
	"github.com/kubemq-io/kubemq-go"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/atomic"
	"log"
	"math/rand"
	"sync"
	"time"
)

type Config struct {
	Address  string
	Queue    string
	Send     int
	Threads  int
	Rounds   int
	AckDelay int
}

var (
	_ = pflag.String("address", "localhost", "kubemq-address")
	_ = pflag.String("queue", "d", "queue destination")
	_ = pflag.Int("send", 1000, "total send messages")
	_ = pflag.Int("threads", 10, "total threads")
	_ = pflag.Int("rounds", 0, "total rounds")
	_ = pflag.Int("ack_delay", 0, "ack delay in seconds")
)

func LoadConfig() (*Config, error) {

	pflag.Parse()
	cfg := &Config{}

	_ = viper.BindEnv("Address", "ADDRESS")
	_ = viper.BindEnv("Send", "SEND")
	_ = viper.BindEnv("Threads", "THREADS")
	_ = viper.BindEnv("Queue", "QUEUE")
	_ = viper.BindEnv("Rounds", "ROUNDS")
	_ = viper.BindEnv("AckDelay", "ACK_DELAY")

	_ = viper.BindPFlag("Address", pflag.CommandLine.Lookup("address"))
	_ = viper.BindPFlag("Queue", pflag.CommandLine.Lookup("queue"))
	_ = viper.BindPFlag("Send", pflag.CommandLine.Lookup("send"))
	_ = viper.BindPFlag("Threads", pflag.CommandLine.Lookup("threads"))
	_ = viper.BindPFlag("Rounds", pflag.CommandLine.Lookup("rounds"))
	_ = viper.BindPFlag("AckDelay", pflag.CommandLine.Lookup("ack_delay"))

	err := viper.Unmarshal(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
func getFloatAvg(list []float64) float64 {
	cnt := 0.0
	for _, val := range list {
		cnt += val
	}
	return cnt / float64(len(list))
}
func getClient(ctx context.Context, host string, port int) *kubemq.QueuesClient {
	c, _ := kubemq.NewQueuesClient(ctx,
		kubemq.WithAddress(host, port),
		kubemq.WithClientId("stream-queue-sender"),
		kubemq.WithTransportType(kubemq.TransportTypeGRPC))
	return c
}
func main() {
	cfg, err := LoadConfig()
	rand.Seed(time.Now().UnixNano())
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var senders []*kubemq.QueuesClient
	senders = append(senders,
		getClient(ctx, cfg.Address, 50000),
		getClient(ctx, cfg.Address, 50000),
		getClient(ctx, cfg.Address, 50000),
	)

	defer func() {
		for i := 0; i < len(senders); i++ {
			_ = senders[i].Close()
		}
	}()
	var roundList []float64
	cnt := 0
	for {
		cnt++
		if cfg.Rounds > 0 && cnt > cfg.Rounds {
			break
		}
		for i := 0; i < cfg.Threads; i++ {
			channel := fmt.Sprintf("%s%d", cfg.Queue, i)
			var batch []*kubemq.QueueMessage
			for i := 0; i < cfg.Send; i++ {
				batch = append(batch, kubemq.NewQueueMessage().
					SetChannel(channel).SetBody([]byte("some-stream-simple-queue-message")))
			}
			_, err = senders[i%len(senders)].Batch(ctx, batch)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf(fmt.Sprintf("%d messages sent to queue %s", cfg.Send, channel))
		}

		startAll := time.Now().UnixNano()
		wg := sync.WaitGroup{}
		wg.Add(cfg.Threads)
		for i := 0; i < cfg.Threads; i++ {
			go func(index int) {
				defer wg.Done()
				cnt := atomic.NewInt32(0)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				receiver, err := kubemq.NewQueuesClient(ctx,
					kubemq.WithAddress(cfg.Address, 50000),
					kubemq.WithClientId(fmt.Sprintf("stream-queue-receiver-%d", index)),
					kubemq.WithAutoReconnect(true),
					kubemq.WithTransportType(kubemq.TransportTypeGRPC))
				if err != nil {
					log.Fatal(err)

				}
				defer receiver.Close()
				channel := fmt.Sprintf("%s%d", cfg.Queue, index)
				done, err := receiver.TransactionStream(ctx, kubemq.NewQueueTransactionMessageRequest().SetChannel(channel).SetVisibilitySeconds(10).SetWaitTimeSeconds(15), func(response *kubemq.QueueTransactionMessageResponse, err error) {
					if err != nil {

					} else {
						cnt.Inc()
						//log.Println (fmt.Sprintf("queue: %s, message seq %d received",response.Message.Channel,response.Message.Attributes.Sequence))
						//t:=rand.Intn(1200-200+1) + 200
						//time.Sleep(time.Duration(t)  *time.Millisecond)
						time.Sleep(time.Duration(cfg.AckDelay) * time.Millisecond)
						err = response.Ack()
						if err != nil {
							//log.Fatal(err)
						}

					}
				})

				//localctx, localCancel := context.WithCancel(ctx)
				//for i := 0; i < 10; i++ {
				//	go func() {
				//		done, err := receiver.Subscribe(localctx, kubemq.NewReceiveQueueMessagesRequest().SetChannel(channel).SetMaxNumberOfMessages(cfg.Send/10).SetWaitTimeSeconds(60), func(response *kubemq.ReceiveQueueMessagesResponse, err error) {
				//			if err != nil {
				//
				//			} else {
				//				cnt.Add(response.MessagesReceived)
				//			}
				//		})
				//		if err != nil {
				//			return
				//		}
				//		<-localctx.Done()
				//		done <- struct{}{}
				//	}()
				//
				//}

				for {
					select {
					case <-time.After(1 * time.Millisecond):
						if cnt.Load() >= int32(cfg.Send) {
							done <- struct{}{}
							return
						}
					}
				}
			}(i)
		}
		wg.Wait()
		avgTime := float64(time.Now().UnixNano()-startAll) / 1e6
		roundList = append(roundList, avgTime)
		fmt.Println(fmt.Sprintf("Round %d, %d Messages received, overall avg time: %f ms", cnt, cfg.Send*cfg.Threads, getFloatAvg(roundList)))
		time.Sleep(time.Second)
	}
	<-ctx.Done()
}
