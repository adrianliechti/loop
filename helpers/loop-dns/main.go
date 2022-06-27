package main

import (
	"errors"
	"log"
	"net"
	"strings"

	"github.com/miekg/dns"
)

const (
	ResolvConf = "/etc/resolv.conf"
)

func main() {
	s, err := New()

	if err != nil {
		panic(err)
	}

	log.Fatal(s.ListenAndServe())
}

type Server struct {
	config *dns.ClientConfig
}

func New() (*Server, error) {
	config, err := dns.ClientConfigFromFile(ResolvConf)

	if err != nil {
		return nil, err
	}

	s := &Server{
		config: config,
	}

	return s, nil
}

func (s *Server) ListenAndServe() error {
	srv := &dns.Server{
		Addr: ":53",
		Net:  "udp",

		Handler: s,
	}

	return srv.ListenAndServe()
}

func (s *Server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	if r.Question[0].Qtype == dns.TypeA {
		domain := r.Question[0].Name

		address, err := s.queryA(domain)

		if err != nil {
			goto FALLBACK
		}

		msg := &dns.Msg{}

		msg.SetReply(r)
		msg.Authoritative = true

		msg.Answer = append(msg.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 10},
			A:   address,
		})

		w.WriteMsg(msg)
		return
	}

FALLBACK:
	msg := &dns.Msg{}
	msg.SetReply(r)

	res, err := s.exchange(msg)

	if err != nil {
	}

	w.WriteMsg(res)
}

func (s *Server) exchange(m *dns.Msg) (*dns.Msg, error) {
	for _, host := range s.config.Servers {
		addr := net.JoinHostPort(host, s.config.Port)

		msg, err := dns.Exchange(m, addr)

		if err != nil {
			continue
		}

		return msg, err
	}

	return nil, errors.New("no dns server available")
}

func (s *Server) queryA(name string) (net.IP, error) {
	domains := []string{
		strings.TrimSuffix(name, ".") + ".",
	}

	for _, s := range s.config.Search {
		d := strings.TrimSuffix(name, ".") + "." + strings.TrimSuffix(s, ".") + "."
		domains = append(domains, d)
	}

	for _, d := range domains {
		msg := &dns.Msg{}

		msg.RecursionDesired = true
		msg.SetQuestion(d, dns.TypeA)

		res, err := s.exchange(msg)

		if err != nil {
			continue
		}

		if len(res.Answer) == 0 {
			continue
		}

		answer := res.Answer[0].(*dns.A)
		return answer.A, nil
	}

	return nil, errors.New("IP not found")
}
