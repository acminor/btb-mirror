/*
 * btb is a program for creating executables that
 * run programs for toolbox containers using toolbox run.
 *
 * Eg. You have a Fedora 35 toolbox container with firefox installed
 * - toolbox -c fedora-toolbox-35 run firefox
 *
 * btb will automatically find all executables accessible from a given
 * toolbox container and will create executables runable from the host
 * with a given prefix. In our earlier example, this would be f35-firefox.
 *
 * These executables will be located inside of a bin folder with a directory prefix.
 * eg. ~/.local/bin/f35/f35-firefox
 *
 * Author: A.C. Minor
 * SPDX identifier: BSD-3-Clause
 */

package main

import "btb/cmd"

func main() {
	cmd.Execute()
}
