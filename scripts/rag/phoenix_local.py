"""Run pinned Phoenix locally using HTTP only.

Phoenix 20.14.0 binds gRPC to all interfaces independently of PHOENIX_HOST.
The experiment uses OTLP/HTTP, so explicitly disable only the unused gRPC
listener through its existing disabled constructor flag.
"""

import sys

from phoenix.server import grpc_server


original_init = grpc_server.GrpcServer.__init__


def http_only_init(self, *args, **kwargs):
    kwargs["disabled"] = True
    original_init(self, *args, **kwargs)


grpc_server.GrpcServer.__init__ = http_only_init

from phoenix.server.main import main

sys.argv = ["phoenix", "serve"]
main()
