import assert from 'node:assert/strict'
import { isDeepStrictEqual } from 'node:util'

export function validateAndroidSerial(serial) {
  if (!/^emulator-\d+$/.test(serial) || serial === 'emulator-5560') throw new Error('Android Starter validation requires an explicitly selected isolated emulator; production demo is forbidden')
  return serial
}

export function assertInstrumentationPassed(output) {
  const statuses = [...output.matchAll(/^INSTRUMENTATION_STATUS_CODE:\s*(-?\d+)\s*$/gm)].map(match => Number(match[1]))
  if (!/^OK \(1 test\)\s*$/m.test(output) || !statuses.includes(0) || statuses.some(code => code < 0)
      || /FAILURES!!!|INSTRUMENTATION_FAILED|INSTRUMENTATION_ABORTED|Process crashed/.test(output)
      || !/^INSTRUMENTATION_CODE:\s*-1\s*$/m.test(output)) throw new Error('Android Starter instrumentation did not report exactly one passed test; inspect android-instrumentation.log')
}

export function messageTexts(message) {
  return typeof message.content === 'string' ? [message.content]
    : Array.isArray(message.content) ? message.content.filter(part => typeof part.text === 'string').map(part => part.text) : []
}

function openingPacket(text) {
  const type = /"type"\s*:\s*"starter_context"/.exec(text)
  if (!type) return undefined
  const start = text.lastIndexOf('{', type.index)
  let depth = 0, quoted = false, escaped = false
  for (let index = start; index >= 0 && index < text.length; index++) {
    const character = text[index]
    if (quoted) {
      if (escaped) escaped = false
      else if (character === '\\') escaped = true
      else if (character === '"') quoted = false
    } else if (character === '"') quoted = true
    else if (character === '{') depth++
    else if (character === '}' && --depth === 0) return JSON.parse(text.slice(start, index + 1))
  }
  throw new Error('Unterminated Starter context in adapter request')
}

export function verifyStarterRequest(requests, starter, assistantSystemPrompt) {
  if (typeof assistantSystemPrompt !== 'string') throw new Error('Original Assistant System is required for replacement verification')
  const matches = requests.filter(request => request.messages?.some(message => message.role === 'user' && messageTexts(message).some(text => text === starter.prompt || text.endsWith('\n' + starter.prompt))))
  if (matches.length !== 1) throw new Error(`Expected one Starter model request, received ${matches.length}`)
  const request = matches[0]
  const systems = request.messages.filter(message => message.role === 'system').flatMap(messageTexts)
  if (!systems.some(text => text.includes(starter.openingSnapshot.systemPrompt))) throw new Error('Published Starter system prompt is absent from the adapter request')
  // Remove the one authored opening before looking for a second source. An
  // operator may legitimately copy/extend the Assistant text inside the opening.
  const remainingSystem = systems.join('\n').replace(starter.openingSnapshot.systemPrompt, '')
  if (assistantSystemPrompt && remainingSystem.includes(assistantSystemPrompt)) throw new Error('Original Assistant System was appended to the Starter opening')
  const packets = request.messages.filter(message => message.role === 'user').flatMap(messageTexts).map(openingPacket).filter(Boolean)
  if (packets.length !== 1) throw new Error(`Expected one Starter context packet, received ${packets.length}`)
  assert.deepEqual(Object.keys(packets[0]).sort(), ['blocks', 'type'], 'Starter packet must not expose internal metadata')
  // The equality assertion is intentionally redacted: only synthetic evidence files contain prompt content.
  if (!isDeepStrictEqual(packets[0].blocks, starter.openingSnapshot.initialContexts.map(block => block.content))) {
    throw new Error('Published Starter background order/content differs from the adapter request')
  }
  return { starterId: starter.starterId, contextIds: starter.openingSnapshot.initialContexts.map(block => block.id), request }
}
