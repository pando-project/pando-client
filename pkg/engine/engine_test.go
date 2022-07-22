package engine

import (
	"context"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestMemEngineCreate(t *testing.T) {
	e, err := New()
	assert.NoError(t, err)
	err = e.Start(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, e.latestMeta, cid.Undef)
	assert.Equal(t, e.pushList, []cid.Cid{})
}

func TestEngineUpdateInfo(t *testing.T) {
	e, err := New(
		WithPublisherKind(DataTransferPublisher),
	)
	assert.NoError(t, err)
	ctx := context.Background()
	err = e.Start(ctx)
	assert.NoError(t, err)
	cid1, err := e.PublishBytesData(ctx, []byte("123"))
	assert.NoError(t, err)

	cc1, err := e.getLatestMetaFromDs(ctx)
	assert.NoError(t, err)
	assert.True(t, cc1.Equals(cid1))

	cid2, err := e.PublishBytesData(ctx, []byte("abc"))
	assert.NoError(t, err)

	cc2, err := e.getLatestMetaFromDs(ctx)
	assert.NoError(t, err)
	assert.True(t, cc2.Equals(cid2))

	cid3, err := e.PublishBytesData(ctx, []byte("123dsa"))
	assert.NoError(t, err)

	cc3, err := e.getLatestMetaFromDs(ctx)
	assert.NoError(t, err)
	assert.True(t, cc3.Equals(cid3))

	assert.Contains(t, e.pushList, cid1)
	assert.Contains(t, e.pushList, cid2)
	assert.Contains(t, e.pushList, cid3)
}

func TestMetaInclusion(t *testing.T) {
	e, err := New(
		WithPublisherKind(DataTransferPublisher),
		WithPandoAPIClient("https://pando-api.kencloud.com", time.Second*10),
	)
	assert.NoError(t, err)
	res, err := e.pandoAPIClient.R().Get("/metadata/inclusion?cid=baguqeeqqw4xw7ahikwhdae2gz4wwiiaxre")
	assert.NoError(t, err)
	t.Log(string(res.Body()))
}
