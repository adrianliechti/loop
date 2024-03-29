package main

import (
	"errors"
	"log"
	"log/slog"
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
	msg, err := s.exchange(r)

	if err != nil {
		msg = new(dns.Msg)
		msg.SetReply(r)

		w.WriteMsg(msg)
		return
	}

	if msg.Rcode == dns.RcodeSuccess {
		slog.Info("handled", "name", r.Question[0].Name, "type", r.Question[0].Qtype)

		w.WriteMsg(msg)
		return
	}

	msg = new(dns.Msg)
	msg.SetReply(r)

	if len(msg.Question) == 1 {
		slog.Info("not handled", "name", r.Question[0].Name, "type", r.Question[0].Qtype)

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
	} else {
		slog.Error("multiple questions not supported")
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
		slog.Info("lookup address", "name", d)

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
