#!/bin/bash -e
rm -f graph.svg
dot -o graph.svg -Tsvg graph.gv
open graph.svg

