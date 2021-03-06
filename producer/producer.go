package producer

import (
	"errors"
	"fmt"
	"time"

	"h12.me/kpax/model"
	"h12.me/kpax/proto"
)

var (
	ErrProduceFailed    = errors.New("produce failed")
	ErrNoValidPartition = errors.New("no valid partition")
)

type P struct {
	LeaderRecoveryTime time.Duration
	RequiredAcks       proto.ProduceAckType
	AckTimeout         time.Duration
	cluster            model.Cluster
	topicPartitioner   *topicPartitioner
}

func New(cluster model.Cluster) *P {
	return &P{
		cluster:            cluster,
		topicPartitioner:   newTopicPartitioner(),
		LeaderRecoveryTime: 60 * time.Second,
		RequiredAcks:       proto.AckLocal,
		AckTimeout:         10 * time.Second,
	}
}

func (p *P) ProduceMessageSet(topic string, messageSet proto.MessageSet) error {
	if len(messageSet) == 0 {
		panic("empty message set")
	}
	key := messageSet[0].Key
	partitioner := p.topicPartitioner.Get(topic)
	if partitioner == nil {
		partitions, err := p.cluster.Partitions(topic)
		if err != nil {
			return err
		}
		partitioner = p.topicPartitioner.Add(topic, partitions)
	}
nextPartition:
	for i := 0; i < partitioner.Count(); i++ {
		partition, err := partitioner.Partition(key)
		if err != nil {
			p.topicPartitioner.Delete(topic)
			return err
		}
		if err := (&proto.Payload{
			Topic:        topic,
			Partition:    partition,
			MessageSet:   messageSet,
			RequiredAcks: p.RequiredAcks,
			AckTimeout:   p.AckTimeout,
		}).Produce(p.cluster); err != nil {
			partitioner.Skip(partition)
			continue nextPartition
		}
		return nil
	}
	return fmt.Errorf("fail to produce to all partitions in %s", topic)
}

func (p *P) Produce(topic string, key, value []byte) error {
	return p.ProduceMessageSet(topic, getMessageSet(key, value))
}

func (p *P) ProduceWithPartition(topic string, partition int32, key, value []byte) error {
	messageSet := getMessageSet(key, value)
	return (&proto.Payload{
		Topic:        topic,
		Partition:    partition,
		MessageSet:   messageSet,
		RequiredAcks: p.RequiredAcks,
		AckTimeout:   p.AckTimeout,
	}).Produce(p.cluster)
}

func getMessageSet(key, value []byte) []proto.OffsetMessage {
	return []proto.OffsetMessage{
		{
			SizedMessage: proto.SizedMessage{CRCMessage: proto.CRCMessage{
				Message: proto.Message{
					Key:   key,
					Value: value,
				},
			}}},
	}
}
