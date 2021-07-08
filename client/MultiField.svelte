<!-- @format -->

<script>
  import {createEventDispatcher} from 'svelte'

  const dispatch = createEventDispatcher()

  const types = ['text-line', 'text-area', 'number', 'bool']
  export let type = types[0]
  export let value = ''
  var parsedvalue

  function change(e) {
    value = e.target.value
    parsedvalue = parseValue(e.target)
    dispatch('change', parsedvalue)
  }

  function toggle() {
    type = types[(types.indexOf(type) + 1) % types.length]
    parsedvalue = parseValue({value, checked: !!parseValue})
    dispatch('change', parsedvalue)
  }

  function parseValue(target) {
    try {
      let json = JSON.parse(target.value)
      if (typeof json === 'object') {
        return json
      }
    } catch (x) {}

    switch (type) {
      case 'text-line':
      case 'text-area':
        return target.value
      case 'number':
        return JSON.parse(target.value)
      case 'bool':
        return target.checked
    }
  }
</script>

<style>
  div {
    display: inline;
  }
  span {
    cursor: pointer;
    cursor: alias;
  }
</style>

<div>
  <span title="click to toggle input type" on:click="{toggle}">⇋</span>
  {#if type == 'text-line'}
  <input type="text" value="{value}" on:input="{change}" />
  {:else if type == 'text-area'}
  <textarea value="{value}" on:input="{change}" />
  {:else if type == 'number'}
  <input type="number" value="{value}" on:input="{change}" />
  {:else if type == 'bool'}
  <input type="checkbox" checked="{!!value}" on:input="{change}" />
  {/if}
</div>
