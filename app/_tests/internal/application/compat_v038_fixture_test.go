package application

// State written by v0.3.8, the compatibility baseline: generated with the
// v0.3.8 code from three shared sing-box nodes, Snell, an IPv4 chain with a
// login, an IPv6 chain and preset rules to both chains and direct. Every
// credential is a random test value and every address a documentation one.
// Regenerate only to add coverage, from the v0.3.8 tag, never from a later
// build: the point is what v0.3.8 wrote.

const compatV038SingBox = `{
  "log": {
    "disabled": false,
    "level": "error",
    "output": "/etc/go-proxy/logs/sing-box.service.log",
    "timestamp": true
  },
  "experimental": {
    "cache_file": {
      "enabled": true,
      "cache_id": "cache.db",
      "path": "/etc/go-proxy/cache/cache.db",
      "store_fakeip": false,
      "store_dns": true
    }
  },
  "dns": {
    "servers": [
      {
        "tag": "public4",
        "type": "https",
        "server": "8.8.8.8",
        "server_port": 443,
        "path": "/dns-query",
        "tls": {
          "enabled": true,
          "server_name": "dns.google"
        }
      },
      {
        "tag": "public6",
        "type": "https",
        "server": "2001:4860:4860::8888",
        "server_port": 443,
        "path": "/dns-query",
        "tls": {
          "enabled": true,
          "server_name": "dns.google"
        }
      },
      {
        "tag": "res1-dns",
        "type": "https",
        "server": "8.8.8.8",
        "server_port": 443,
        "path": "/dns-query",
        "tls": {
          "enabled": true,
          "server_name": "dns.google"
        },
        "detour": "res1"
      },
      {
        "tag": "res6-dns",
        "type": "https",
        "server": "2001:4860:4860::8888",
        "server_port": 443,
        "path": "/dns-query",
        "tls": {
          "enabled": true,
          "server_name": "dns.google"
        },
        "detour": "res6"
      }
    ],
    "rules": [
      {
        "action": "route",
        "server": "public6",
        "auth_user": [
          "alice"
        ],
        "rule_set": [
          "geosite-category-ads-all",
          "geosite-netflix"
        ]
      },
      {
        "action": "predefined",
        "auth_user": [
          "alice"
        ],
        "query_type": [
          "AAAA"
        ],
        "rule_set": [
          "geosite-google",
          "geosite-xai"
        ]
      },
      {
        "action": "route",
        "server": "res1-dns",
        "auth_user": [
          "alice"
        ],
        "rule_set": [
          "geosite-google",
          "geosite-xai"
        ]
      },
      {
        "action": "predefined",
        "auth_user": [
          "alice",
          "bob"
        ],
        "query_type": [
          "AAAA"
        ],
        "rule_set": [
          "geosite-anthropic",
          "geosite-openai"
        ]
      },
      {
        "action": "route",
        "server": "res1-dns",
        "auth_user": [
          "alice",
          "bob"
        ],
        "rule_set": [
          "geosite-anthropic",
          "geosite-openai"
        ]
      },
      {
        "action": "predefined",
        "auth_user": [
          "bob"
        ],
        "query_type": [
          "A"
        ],
        "rule_set": [
          "geosite-category-ai-!cn",
          "geosite-telegram"
        ]
      },
      {
        "action": "route",
        "server": "res6-dns",
        "auth_user": [
          "bob"
        ],
        "rule_set": [
          "geosite-category-ai-!cn",
          "geosite-telegram"
        ]
      },
      {
        "action": "evaluate",
        "server": "public6",
        "tag": "public6-geoip-4",
        "auth_user": [
          "alice"
        ]
      },
      {
        "action": "respond",
        "auth_user": [
          "alice"
        ],
        "rule_set": [
          "geoip-netflix"
        ],
        "match_response": "public6-geoip-4"
      },
      {
        "action": "evaluate",
        "server": "res1-dns",
        "tag": "res1-dns-geoip-5",
        "auth_user": [
          "alice"
        ],
        "query_type": [
          "A"
        ]
      },
      {
        "action": "respond",
        "auth_user": [
          "alice"
        ],
        "rule_set": [
          "geoip-google"
        ],
        "match_response": "res1-dns-geoip-5"
      },
      {
        "action": "evaluate",
        "server": "res6-dns",
        "tag": "res6-dns-geoip-6",
        "auth_user": [
          "bob"
        ],
        "query_type": [
          "AAAA"
        ]
      },
      {
        "action": "respond",
        "auth_user": [
          "bob"
        ],
        "rule_set": [
          "geoip-ai",
          "geoip-telegram"
        ],
        "match_response": "res6-dns-geoip-6"
      }
    ],
    "final": "public6",
    "strategy": "prefer_ipv6",
    "reverse_mapping": true,
    "cache_capacity": 8192
  },
  "http_clients": [
    {
      "tag": "direct",
      "detour": "direct"
    }
  ],
  "inbounds": [
    {
      "type": "vless",
      "tag": "vless_reality_443",
      "listen": "::",
      "listen_port": 443,
      "users": [
        {
          "name": "alice",
          "uuid": "2cc27515-f363-423d-b232-b933b68580c7",
          "flow": "xtls-rprx-vision"
        },
        {
          "name": "bob",
          "uuid": "b8eb3607-b9a4-40df-a7ea-6f87de9a8181",
          "flow": "xtls-rprx-vision"
        }
      ],
      "tls": {
        "enabled": true,
        "server_name": "www.example.com",
        "reality": {
          "enabled": true,
          "handshake": {
            "server": "www.example.com",
            "server_port": 443
          },
          "private_key": "qwPvqhJDOUwbPNEE7umj8-xHdByL47GG9yobFfPBYWQ",
          "short_id": [
            "17f6008c"
          ]
        }
      }
    },
    {
      "type": "tuic",
      "tag": "tuic_25020",
      "listen": "::",
      "listen_port": 25020,
      "users": [
        {
          "name": "alice",
          "uuid": "c8b1d384-2118-413a-81e6-249d0e13d99e",
          "password": "7mKw9ZW1hcIipmeH"
        },
        {
          "name": "bob",
          "uuid": "5984b832-24c5-4669-be16-d1b0e8a3bf20",
          "password": "iyGL7bPMZHBMhZWy"
        }
      ],
      "tls": {
        "enabled": true,
        "server_name": "node.example.com",
        "alpn": [
          "h3"
        ]
      },
      "congestion_control": "bbr"
    },
    {
      "type": "anytls",
      "tag": "anytls_2053",
      "listen": "::",
      "listen_port": 2053,
      "users": [
        {
          "name": "alice",
          "password": "8MU6EkvTGZ2mYmL1"
        },
        {
          "name": "bob",
          "password": "HvwoFuy4DCKpn1N9"
        }
      ],
      "tls": {
        "enabled": true,
        "server_name": "node.example.com"
      }
    }
  ],
  "outbounds": [
    {
      "type": "direct",
      "tag": "direct",
      "domain_resolver": {
        "server": "public6",
        "strategy": "prefer_ipv6"
      }
    },
    {
      "type": "socks",
      "tag": "res1",
      "server": "198.51.100.7",
      "server_port": 1080,
      "version": "5",
      "username": "user1",
      "password": "secret-1"
    },
    {
      "type": "socks",
      "tag": "res6",
      "server": "2001:db8::7",
      "server_port": 1080,
      "version": "5"
    }
  ],
  "route": {
    "final": "direct",
    "default_domain_resolver": "public6",
    "default_http_client": "direct",
    "rules": [
      {
        "action": "sniff",
        "sniffer": [
          "http",
          "tls",
          "quic",
          "dns"
        ]
      },
      {
        "action": "hijack-dns",
        "protocol": "dns"
      },
      {
        "action": "route",
        "outbound": "direct",
        "ip_is_private": true
      },
      {
        "action": "route",
        "outbound": "direct",
        "domain_suffix": [
          "gstatic.com"
        ]
      },
      {
        "action": "route",
        "outbound": "direct",
        "auth_user": [
          "alice"
        ],
        "rule_set": [
          "geosite-category-ads-all",
          "geosite-netflix"
        ]
      },
      {
        "action": "route",
        "outbound": "res1",
        "auth_user": [
          "alice"
        ],
        "rule_set": [
          "geosite-google",
          "geosite-xai"
        ]
      },
      {
        "action": "route",
        "outbound": "res1",
        "auth_user": [
          "alice",
          "bob"
        ],
        "rule_set": [
          "geosite-anthropic",
          "geosite-openai"
        ]
      },
      {
        "action": "route",
        "outbound": "res6",
        "auth_user": [
          "bob"
        ],
        "rule_set": [
          "geosite-category-ai-!cn",
          "geosite-telegram"
        ]
      },
      {
        "action": "route",
        "outbound": "direct",
        "auth_user": [
          "alice"
        ],
        "rule_set": [
          "geoip-netflix"
        ]
      },
      {
        "action": "route",
        "outbound": "res1",
        "auth_user": [
          "alice"
        ],
        "rule_set": [
          "geoip-google"
        ]
      },
      {
        "action": "route",
        "outbound": "res6",
        "auth_user": [
          "bob"
        ],
        "rule_set": [
          "geoip-ai",
          "geoip-telegram"
        ]
      }
    ],
    "rule_set": [
      {
        "tag": "geosite-openai",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/openai.srs"
      },
      {
        "tag": "geosite-anthropic",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/anthropic.srs"
      },
      {
        "tag": "geosite-category-ai-!cn",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/category-ai-!cn.srs"
      },
      {
        "tag": "geoip-ai",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/ai.srs"
      },
      {
        "tag": "geosite-google",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/google.srs"
      },
      {
        "tag": "geosite-netflix",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/netflix.srs"
      },
      {
        "tag": "geosite-disney",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/disney.srs"
      },
      {
        "tag": "geosite-mytvsuper",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/mytvsuper.srs"
      },
      {
        "tag": "geosite-youtube",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/youtube.srs"
      },
      {
        "tag": "geosite-spotify",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/spotify.srs"
      },
      {
        "tag": "geosite-tiktok",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/tiktok.srs"
      },
      {
        "tag": "geosite-telegram",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/telegram.srs"
      },
      {
        "tag": "geosite-category-ads-all",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/category-ads-all.srs"
      },
      {
        "tag": "geosite-twitter",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/twitter.srs"
      },
      {
        "tag": "geosite-whatsapp",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/whatsapp.srs"
      },
      {
        "tag": "geosite-facebook",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/facebook.srs"
      },
      {
        "tag": "geosite-discord",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/discord.srs"
      },
      {
        "tag": "geosite-instagram",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/instagram.srs"
      },
      {
        "tag": "geosite-reddit",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/reddit.srs"
      },
      {
        "tag": "geosite-linkedin",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/linkedin.srs"
      },
      {
        "tag": "geosite-paypal",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/paypal.srs"
      },
      {
        "tag": "geosite-microsoft",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/microsoft.srs"
      },
      {
        "tag": "geosite-xai",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/xai.srs"
      },
      {
        "tag": "geosite-meta",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/meta.srs"
      },
      {
        "tag": "geosite-messenger",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/messenger.srs"
      },
      {
        "tag": "geosite-github",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/github.srs"
      },
      {
        "tag": "geosite-spacex",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/spacex.srs"
      },
      {
        "tag": "geoip-google",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/google.srs"
      },
      {
        "tag": "geoip-netflix",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/netflix.srs"
      },
      {
        "tag": "geoip-twitter",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/twitter.srs"
      },
      {
        "tag": "geoip-telegram",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/telegram.srs"
      },
      {
        "tag": "geoip-facebook",
        "type": "remote",
        "format": "binary",
        "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/facebook.srs"
      }
    ]
  }
}
`

const compatV038UserMeta = `{
  "name": {
    "anytls|anytls_2053|8MU6EkvTGZ2mYmL1": "alice",
    "anytls|anytls_2053|HvwoFuy4DCKpn1N9": "bob",
    "snell|snell-v6|IeGYD2mcqk3yA237tyBuYsNU2BlcfKrh": "alice",
    "tuic|tuic_25020|5984b832-24c5-4669-be16-d1b0e8a3bf20": "bob",
    "tuic|tuic_25020|c8b1d384-2118-413a-81e6-249d0e13d99e": "alice",
    "vless|vless_reality_443|2cc27515-f363-423d-b232-b933b68580c7": "alice",
    "vless|vless_reality_443|b8eb3607-b9a4-40df-a7ea-6f87de9a8181": "bob"
  },
  "groups": {
    "all": [
      "alice",
      "bob"
    ]
  },
  "chain_strategy": {
    "res1": "ipv4_only",
    "res6": "ipv6_only"
  }
}
`

const compatV038UserRoutes = `[
  {
    "outbound": "res1",
    "auth_user": [
      "alice"
    ],
    "rule_set": [
      "geosite-openai"
    ]
  },
  {
    "outbound": "res1",
    "auth_user": [
      "alice"
    ],
    "rule_set": [
      "geosite-anthropic"
    ]
  },
  {
    "outbound": "res1",
    "auth_user": [
      "alice"
    ],
    "rule_set": [
      "geosite-google",
      "geoip-google"
    ]
  },
  {
    "outbound": "res1",
    "auth_user": [
      "alice"
    ],
    "rule_set": [
      "geosite-xai"
    ]
  },
  {
    "outbound": "direct",
    "auth_user": [
      "alice"
    ],
    "rule_set": [
      "geosite-netflix",
      "geoip-netflix"
    ]
  },
  {
    "outbound": "direct",
    "auth_user": [
      "alice"
    ],
    "rule_set": [
      "geosite-category-ads-all"
    ]
  },
  {
    "outbound": "res1",
    "auth_user": [
      "bob"
    ],
    "rule_set": [
      "geosite-openai"
    ]
  },
  {
    "outbound": "res1",
    "auth_user": [
      "bob"
    ],
    "rule_set": [
      "geosite-anthropic"
    ]
  },
  {
    "outbound": "res6",
    "auth_user": [
      "bob"
    ],
    "rule_set": [
      "geosite-category-ai-!cn",
      "geoip-ai"
    ]
  },
  {
    "outbound": "res6",
    "auth_user": [
      "bob"
    ],
    "rule_set": [
      "geosite-telegram",
      "geoip-telegram"
    ]
  }
]
`

const compatV038Snell = `[snell-server]
listen = 0.0.0.0:1443,[::]:1443
psk = IeGYD2mcqk3yA237tyBuYsNU2BlcfKrh
mode = default
dns-ip-preference = default
`
