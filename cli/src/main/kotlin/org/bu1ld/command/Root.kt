package org.bu1ld.command

import com.github.ajalt.clikt.core.CliktCommand
import com.github.ajalt.clikt.core.PrintHelpMessage

class Root : CliktCommand() {

    override fun run() {
        throw PrintHelpMessage(currentContext) as Throwable
    }
}
