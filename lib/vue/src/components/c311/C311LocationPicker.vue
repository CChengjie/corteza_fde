<template>
  <fieldset class="c311-location-picker" data-c311-location-picker>
    <legend>{{ translate('portal.submit.location', 'Location') }}</legend>
    <div class="form-group">
      <label for="c311-location-address">{{ translate('field.address', 'Address') }}</label>
      <div class="input-group">
        <input
          id="c311-location-address"
          class="form-control"
          autocomplete="address-line1"
          :value="address"
          :aria-invalid="addressInvalid ? 'true' : 'false'"
          @input="$emit('update:address', $event.target.value)"
        >
        <div class="input-group-append">
          <button class="btn btn-outline-primary" type="button" data-c311-action="geocode-address" :disabled="geocodeLoading || !address" @click="$emit('geocode')">
            {{ geocodeLoading ? translate('status.geocoding', 'Finding address...') : translate('action.findAddress', 'Find address') }}
          </button>
        </div>
      </div>
    </div>
    <p v-if="geocodeError" class="alert alert-danger" role="alert" data-c311-map-error>
      {{ geocodeError.message || translate('error.mapUnavailable', 'The map service could not process this address.') }}
      <button v-if="geocodeError.retryable" class="btn btn-link p-0 ml-1" type="button" data-c311-action="retry-geocode" @click="$emit('retry')">
        {{ translate('action.retry', 'Try again') }}
      </button>
    </p>
    <p v-if="normalizedAddress" class="text-muted" data-c311-normalized-address>{{ normalizedAddress }}</p>
    <div v-if="hasCoordinates" class="c311-location-map" data-c311-map>
      <c-map
        v-if="mode === 'http'"
        class="c311-location-map__leaflet"
        :hide-geo-search="true"
        :markers="[{ value: [latitude, longitude], title: normalizedAddress || address }]"
        :map="{ center: [latitude, longitude], zoom: 15 }"
        @on-map-click="onMapClick"
      />
      <div v-else class="c311-location-map__mock" role="application" :aria-label="translate('map.label', 'Map location')">
        <span
          class="c311-location-map__marker"
          role="button"
          tabindex="0"
          data-c311-map-marker
          :style="markerStyle"
          :aria-label="translate('map.marker', 'Move location marker')"
          @keydown="onMarkerKeydown"
          @click="$emit('focus-marker')"
        >
          <span class="sr-only">{{ latitude }}, {{ longitude }}</span>
        </span>
        <span class="sr-only">{{ translate('map.keyboardHelp', 'Use the arrow keys or the controls below to adjust the marker.') }}</span>
      </div>
      <div class="c311-location-map__controls" role="group" :aria-label="translate('map.adjust', 'Adjust marker')">
        <button class="btn btn-sm btn-outline-secondary" type="button" data-c311-action="move-marker-north" :aria-label="translate('map.moveNorth', 'Move marker north')" @click="$emit('move-marker', { latitude: 0.0001, longitude: 0 })">+</button>
        <button class="btn btn-sm btn-outline-secondary" type="button" data-c311-action="move-marker-south" :aria-label="translate('map.moveSouth', 'Move marker south')" @click="$emit('move-marker', { latitude: -0.0001, longitude: 0 })">-</button>
        <button class="btn btn-sm btn-outline-secondary" type="button" data-c311-action="move-marker-east" :aria-label="translate('map.moveEast', 'Move marker east')" @click="$emit('move-marker', { latitude: 0, longitude: 0.0001 })">+</button>
        <button class="btn btn-sm btn-outline-secondary" type="button" data-c311-action="move-marker-west" :aria-label="translate('map.moveWest', 'Move marker west')" @click="$emit('move-marker', { latitude: 0, longitude: -0.0001 })">-</button>
      </div>
    </div>
    <div class="c311-location-picker__coordinates">
      <div class="form-group">
        <label for="c311-location-latitude">{{ translate('field.latitude', 'Latitude') }}</label>
        <input id="c311-location-latitude" class="form-control" type="number" min="-90" max="90" step="0.0001" :value="latitude" :aria-invalid="latitudeInvalid ? 'true' : 'false'" @input="$emit('update:latitude', $event.target.value)">
      </div>
      <div class="form-group">
        <label for="c311-location-longitude">{{ translate('field.longitude', 'Longitude') }}</label>
        <input id="c311-location-longitude" class="form-control" type="number" min="-180" max="180" step="0.0001" :value="longitude" :aria-invalid="longitudeInvalid ? 'true' : 'false'" @input="$emit('update:longitude', $event.target.value)">
      </div>
    </div>
    <button class="btn btn-outline-primary" type="button" data-c311-action="confirm-location" :disabled="!hasCoordinates" @click="$emit('confirm')">
      {{ confirmed ? translate('status.locationConfirmed', 'Location confirmed') : translate('action.confirmLocation', 'Confirm location') }}
    </button>
    <span v-if="confirmed" class="text-success ml-2" role="status" data-c311-location-confirmed>{{ translate('status.locationConfirmed', 'Location confirmed') }}</span>
  </fieldset>
</template>

<script>
import CMap from '../map/CMap.vue'

export default {
  name: 'C311LocationPicker',
  components: { CMap },
  props: {
    address: { type: String, default: '' },
    latitude: { type: [Number, String], default: null },
    longitude: { type: [Number, String], default: null },
    confirmed: Boolean,
    mode: { type: String, default: 'mock' },
    geocodeLoading: Boolean,
    geocodeError: { type: [Object, Error], default: null },
    normalizedAddress: { type: String, default: '' },
    addressInvalid: Boolean,
    latitudeInvalid: Boolean,
    longitudeInvalid: Boolean,
  },
  computed: {
    hasCoordinates () {
      return this.latitude !== null && this.latitude !== '' && this.longitude !== null && this.longitude !== '' && Number.isFinite(Number(this.latitude)) && Number.isFinite(Number(this.longitude))
    },
    markerStyle () {
      const left = Math.max(8, Math.min(92, ((Number(this.longitude) + 180) / 360) * 100))
      const top = Math.max(8, Math.min(92, ((90 - Number(this.latitude)) / 180) * 100))
      return { left: `${left}%`, top: `${top}%` }
    },
  },
  methods: {
    translate (key, fallback) {
      const translated = this.$t?.(`c311:${key}`)
      return translated && translated !== `c311:${key}` && translated !== key ? translated : fallback
    },
    onMarkerKeydown (event) {
      const deltas = { ArrowUp: { latitude: 0.0001, longitude: 0 }, ArrowDown: { latitude: -0.0001, longitude: 0 }, ArrowRight: { latitude: 0, longitude: 0.0001 }, ArrowLeft: { latitude: 0, longitude: -0.0001 } }
      if (!deltas[event.key]) return
      event.preventDefault()
      this.$emit('move-marker', deltas[event.key])
    },
    onMapClick (event) {
      const latlng = event?.latlng || event
      if (latlng && Number.isFinite(Number(latlng.lat)) && Number.isFinite(Number(latlng.lng))) this.$emit('set-coordinates', { latitude: Number(latlng.lat), longitude: Number(latlng.lng) })
    },
  },
}
</script>

<style scoped>
.c311-location-picker__coordinates { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 1rem; }
.c311-location-map { position: relative; min-height: 12rem; margin-bottom: 1rem; border: 1px solid #ced4da; overflow: hidden; }
.c311-location-map__mock { position: relative; min-height: 12rem; background: repeating-linear-gradient(0deg, #eef4f7 0, #eef4f7 1.5rem, #dce8ed 1.5rem, #dce8ed 1.6rem), repeating-linear-gradient(90deg, transparent 0, transparent 2rem, rgba(21, 94, 239, .12) 2rem, rgba(21, 94, 239, .12) 2.1rem); }
.c311-location-map__marker { position: absolute; width: 1.25rem; height: 1.25rem; border-radius: 50%; transform: translate(-50%, -50%); background: #155eef; box-shadow: 0 0 0 3px #fff, 0 0 0 5px #155eef; cursor: move; }
.c311-location-map__controls { display: flex; gap: .25rem; position: absolute; right: .5rem; bottom: .5rem; background: rgba(255, 255, 255, .9); padding: .25rem; }
@media (max-width: 767px) { .c311-location-picker__coordinates { grid-template-columns: 1fr; gap: 0; } }
</style>
