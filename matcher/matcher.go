package matcher

import (
	"fmt"
	"github.com/fmstephe/matching_engine/trade"
)

type M struct {
	matchTrees trade.MatchTrees // No constructor required
	slab       *trade.Slab
	rb         *ResponseBuffer
}

func NewMatcher(slabSize int, rb *ResponseBuffer) *M {
	slab := trade.NewSlab(slabSize)
	return &M{slab: slab, rb: rb}
}

func (m *M) Submit(in *trade.Order) {
	o := m.slab.Malloc()
	in.CopyInto(o)
	switch o.Kind {
	case trade.BUY:
		m.addBuy(o)
	case trade.SELL:
		m.addSell(o)
	case trade.DELETE:
		m.remove(o)
	default:
		panic(fmt.Sprintf("OrderKind %#v not supported", o.Kind))
	}
}

func (m *M) addBuy(b *trade.Order) {
	if b.Price() == trade.MARKET_PRICE {
		panic("It is illegal to submit a buy at market price")
	}
	if !m.fillableBuy(b) {
		m.matchTrees.PushBuy(b)
	}
}

func (m *M) addSell(s *trade.Order) {
	if !m.fillableSell(s) {
		m.matchTrees.PushSell(s)
	}
}

func (m *M) remove(o *trade.Order) {
	ro := m.matchTrees.Pop(o)
	if ro != nil {
		completeDelete(m.rb, trade.DELETED, ro)
		m.slab.Free(ro)
	} else {
		completeDelete(m.rb, trade.NOT_DELETED, o)
	}
	m.slab.Free(o)
}

func (m *M) fillableBuy(b *trade.Order) bool {
	for {
		s := m.matchTrees.PeekSell()
		if s == nil {
			return false
		}
		if b.Price() >= s.Price() {
			if b.Amount > s.Amount {
				amount := s.Amount
				price := price(b.Price(), s.Price())
				m.slab.Free(m.matchTrees.PopSell())
				b.Amount -= amount
				completeTrade(m.rb, trade.PARTIAL, trade.FULL, b, s, price, amount)
				continue
			}
			if s.Amount > b.Amount {
				amount := b.Amount
				price := price(b.Price(), s.Price())
				s.Amount -= amount
				completeTrade(m.rb, trade.FULL, trade.PARTIAL, b, s, price, amount)
				m.slab.Free(b)
				return true // The buy has been used up
			}
			if s.Amount == b.Amount {
				amount := b.Amount
				price := price(b.Price(), s.Price())
				completeTrade(m.rb, trade.FULL, trade.FULL, b, s, price, amount)
				m.slab.Free(m.matchTrees.PopSell())
				m.slab.Free(b)
				return true // The buy has been used up
			}
		} else {
			return false
		}
	}
	panic("Unreachable")
}

func (m *M) fillableSell(s *trade.Order) bool {
	for {
		b := m.matchTrees.PeekBuy()
		if b == nil {
			return false
		}
		if b.Price() >= s.Price() {
			if b.Amount > s.Amount {
				amount := s.Amount
				price := price(b.Price(), s.Price())
				b.Amount -= amount
				completeTrade(m.rb, trade.PARTIAL, trade.FULL, b, s, price, amount)
				m.slab.Free(s)
				return true // The sell has been used up
			}
			if s.Amount > b.Amount {
				amount := b.Amount
				price := price(b.Price(), s.Price())
				s.Amount -= amount
				completeTrade(m.rb, trade.PARTIAL, trade.FULL, b, s, price, amount)
				m.slab.Free(m.matchTrees.PopBuy())
				continue
			}
			if s.Amount == b.Amount {
				amount := b.Amount
				price := price(b.Price(), s.Price())
				completeTrade(m.rb, trade.FULL, trade.FULL, b, s, price, amount)
				m.slab.Free(m.matchTrees.PopBuy())
				m.slab.Free(s)
				return true // The sell has been used up
			}
		} else {
			return false
		}
	}
	panic("Unreachable")
}

func price(bPrice, sPrice int64) int64 {
	if sPrice == trade.MARKET_PRICE {
		return bPrice
	}
	d := bPrice - sPrice
	return sPrice + (d >> 1)
}

func completeTrade(rb *ResponseBuffer, brk, srk trade.ResponseKind, b, s *trade.Order, price int64, amount uint32) {
	br := rb.getForWrite()
	sr := rb.getForWrite()
	br.WriteTrade(brk, -price, amount, b.TraderId(), b.TradeId(), s.TraderId())
	sr.WriteTrade(srk, price, amount, s.TraderId(), s.TradeId(), b.TraderId())
}

func completeDelete(rb *ResponseBuffer, rk trade.ResponseKind, d *trade.Order) {
	r := rb.getForWrite()
	r.WriteDelete(rk, d.TraderId(), d.TradeId())
}
