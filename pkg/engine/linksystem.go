package engine

import (
	"bytes"
	"io"

	//provider "github.com/filecoin-project/index-provider"
	//"github.com/filecoin-project/storetheindex/api/v0/ingest/schema"
	"github.com/ipfs/go-datastore"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagjson"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
)

// Creates the main engine linksystem.
func (e *Engine) mkLinkSystem() ipld.LinkSystem {
	lsys := cidlink.DefaultLinkSystem()
	lsys.StorageReadOpener = func(lctx ipld.LinkContext, lnk ipld.Link) (io.Reader, error) {

		ctx := lctx.Ctx
		c := lnk.(cidlink.Link).Cid
		logger.Debugf("Triggered ReadOpener from engine's linksystem with cid (%s)", c)

		// Get the node from main datastore. If it is in the
		// main datastore it means it is an advertisement.
		val, err := e.ds.Get(ctx, datastore.NewKey(c.String()))
		if err != nil {
			if err == datastore.ErrNotFound {
				return nil, err
			}
			logger.Errorf("Error getting object from datastore in linksystem: %s", err)
			return nil, err
		}

		// If data was retrieved from the datastore, this may be a metadata.
		if len(val) != 0 {
			// Decode the node to check its type to see if it is a metadata.
			n, err := decodeIPLDNode(bytes.NewBuffer(val))
			if err != nil {
				logger.Errorf("Could not decode IPLD node for potential advertisement: %s", err)
				return nil, err
			}
			// If this was an advertisement, then return it.
			if isMetadata(n) {
				logger.Debugw("Retrieved advertisement from datastore", "cid", c, "size", len(val))
				return bytes.NewBuffer(val), nil
			}
			logger.Debugw("Retrieved non-advertisement object from datastore", "cid", c, "size", len(val))
		}

		return bytes.NewBuffer(val), nil
	}
	lsys.StorageWriteOpener = func(lctx ipld.LinkContext) (io.Writer, ipld.BlockWriteCommitter, error) {
		buf := bytes.NewBuffer(nil)
		return buf, func(lnk ipld.Link) error {
			c := lnk.(cidlink.Link).Cid
			return e.ds.Put(lctx.Ctx, datastore.NewKey(c.String()), buf.Bytes())
		}, nil
	}
	return lsys
}

// vanillaLinkSystem plainly loads and stores from engine datastore.
//
// This is used to plainly load and store links without the complex
// logic of the main linksystem. This is mainly used to retrieve
// stored advertisements through the link from the main blockstore.
func (e *Engine) vanillaLinkSystem() ipld.LinkSystem {
	lsys := cidlink.DefaultLinkSystem()
	lsys.StorageReadOpener = func(lctx ipld.LinkContext, lnk ipld.Link) (io.Reader, error) {
		c := lnk.(cidlink.Link).Cid
		val, err := e.ds.Get(lctx.Ctx, datastore.NewKey(c.String()))
		if err != nil {
			return nil, err
		}
		return bytes.NewBuffer(val), nil
	}
	lsys.StorageWriteOpener = func(lctx ipld.LinkContext) (io.Writer, ipld.BlockWriteCommitter, error) {
		buf := bytes.NewBuffer(nil)
		return buf, func(lnk ipld.Link) error {
			c := lnk.(cidlink.Link).Cid
			return e.ds.Put(lctx.Ctx, datastore.NewKey(c.String()), buf.Bytes())
		}, nil
	}
	return lsys
}

// decodeIPLDNode reads the content of the given reader fully as an IPLD node.
func decodeIPLDNode(r io.Reader) (ipld.Node, error) {
	nb := basicnode.Prototype.Any.NewBuilder()
	err := dagjson.Decode(nb, r)
	if err != nil {
		return nil, err
	}
	return nb.Build(), nil
}

// isMetadata loosely checks if an IPLD node is an advertisement or an index.
// This is done simply by checking if `Signature` filed is present.
func isMetadata(n ipld.Node) bool {
	signature, _ := n.LookupByString("Signature")
	provider, _ := n.LookupByString("Provider")
	payload, _ := n.LookupByString("Payload")
	return signature != nil && payload != nil && provider != nil
}
