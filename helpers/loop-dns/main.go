package main

import (
	"errors"
	"net"
	"strings"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
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
	msg, err := s.exchange(r)

	if err != nil {
		msg = new(dns.Msg)
		msg.SetReply(r)

		w.WriteMsg(msg)
		return
	}

	if msg.Rcode == dns.RcodeSuccess {
		w.WriteMsg(msg)
		return
	}

	msg = new(dns.Msg)
	msg.SetReply(r)

	if len(msg.Question) == 1 {
		log.WithFields(log.Fields{
			"name": r.Question[0].Name,
			"type": r.Question[0].Qtype,
		}).Info("not handled")

		if r.Question[0].Qtype == dns.TypeA {
			name := r.Question[0].Name

			if addr, err := s.queryA(name); err == nil {
				msg.Authoritative = true

				msg.Answer = append(msg.Answer, &dns.A{
					Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 10},
					A:   addr,
				})
			}
		}
	}

	w.WriteMsg(msg)
}

func (s *Server) exchange(m *dns.Msg) (*dns.Msg, error) {
	for _, host := range s.config.Servers {
		addr := net.JoinHostPort(host, s.config.Port)

		msg, err := dns.Exchange(m, addr)

		if err != nil {
			continue
		}

		return msg, nil
	}

	return nil, errors.New("failed to exchange")
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
		log.WithFields(log.Fields{
			"name": d,
		}).Info("lookup address")

		msg := &dns.Msg{}

		msg.RecursionDesired = true
		msg.SetQuestion(d, dns.TypeA)

		res, err := s.exchange(msg)

		if err != nil || len(res.Answer) == 0 {
			continue
		}

		answer, ok := res.Answer[0].(*dns.A)

		if !ok || answer.A == nil {
			continue
		}

		return answer.A, nil
	}

	return nil, errors.New("IP not found")
}
