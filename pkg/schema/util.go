package schema

import (
	"context"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	sc "github.com/kenlabs/pando/pkg/types/schema"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

func NewMetaWithBytesPayload(payload []byte, provider peer.ID, signKey crypto.PrivKey, prev datamodel.Link) (*sc.Metadata, error) {
	pnode := basicnode.NewBytes(payload)
	return NewMetaWithPayloadNode(pnode, provider, signKey, prev)
}

func NewMetaWithPayloadNode(payload datamodel.Node, provider peer.ID, signKey crypto.PrivKey, prev datamodel.Link) (*sc.Metadata, error) {
	meta := &sc.Metadata{
		Provider: provider.String(),
		Payload:  payload,
	}
	if prev == nil {
		meta.PreviousID = nil
	} else {
		meta.PreviousID = &prev
	}

	sig, err := sc.SignWithPrivky(signKey, meta)
	if err != nil {
		return nil, err
	}

	// Add signature
	meta.Signature = sig
	return meta, nil
}

//func NewMetadataWithLink(payload []byte, provider peer.ID, signKey crypto.PrivKey, link datamodel.Link) (*sc.Metadata, error) {
//	if link == nil {
//		return nil, fmt.Errorf("nil previous meta link")
//	}
//
//	pnode := basicnode.NewBytes(payload)
//	meta := &sc.Metadata{
//		PreviousID: &link,
//		Provider:   provider.String(),
//		Payload:    pnode,
//	}
//
//	sig, err := sc.SignWithPrivky(signKey, meta)
//	if err != nil {
//		return nil, err
//	}
//
//	// Add signature
//	meta.Signature = sig
//
//	return meta, nil
//}

func MetadataLink(lsys ipld.LinkSystem, metadata *sc.Metadata) (datamodel.Link, error) {
	mnode, err := metadata.ToNode()
	if err != nil {
		return cidlink.Link{}, err
	}
	lnk, err := lsys.Store(metadata.LinkContext(context.Background()), sc.LinkProto, mnode)
	if err != nil {
		return nil, err
	}

	return lnk, nil
}
