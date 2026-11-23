// @ts-check
// Demo page logic: price calculation and form handling. Tracking lives in tracker.js.
// Prices come from data-price (pizza, toppings) and data-factor (size) in index.html.
(function () {
  'use strict';

  const PROMO_CODE = 'PIZZA10';
  const PROMO_DISCOUNT = 0.1;

  // Elements from index.html; the casts tell the type checker their exact types.
  const form = /** @type {HTMLFormElement} */ (document.getElementById('order-form'));
  const pizza = /** @type {HTMLSelectElement} */ (document.getElementById('pizza-select'));
  const slider = /** @type {HTMLInputElement} */ (document.getElementById('quantity-slider'));
  const quantityOutput = /** @type {HTMLOutputElement} */ (document.getElementById('quantity-output'));
  const promoInput = /** @type {HTMLInputElement} */ (document.getElementById('promo-code'));
  const promoStatus = /** @type {HTMLElement} */ (document.getElementById('promo-status'));
  const total = /** @type {HTMLElement} */ (document.getElementById('order-total'));
  const orderStatus = /** @type {HTMLElement} */ (document.getElementById('order-status'));
  const nameInput = /** @type {HTMLInputElement} */ (document.getElementById('customer-name'));
  const addressInput = /** @type {HTMLInputElement} */ (document.getElementById('delivery-address'));
  const minusButton = /** @type {HTMLButtonElement} */ (document.getElementById('quantity-minus-button'));
  const plusButton = /** @type {HTMLButtonElement} */ (document.getElementById('quantity-plus-button'));
  const promoButton = /** @type {HTMLButtonElement} */ (document.getElementById('promo-apply-button'));
  const resetButton = /** @type {HTMLButtonElement} */ (document.getElementById('reset-button'));

  let promoApplied = false;

  /**
   * Order price in rubles: (pizza * size factor + toppings) * quantity, minus the promo discount.
   * @returns {number}
   */
  function calcTotal() {
    const base = Number(pizza.selectedOptions[0].dataset.price);
    const size = /** @type {HTMLInputElement} */ (form.querySelector('input[name="size"]:checked'));
    /** @type {NodeListOf<HTMLInputElement>} */
    const checked = form.querySelectorAll('input[name="toppings"]:checked');
    const toppings = Array.from(checked)
      .reduce((sum, el) => sum + Number(el.dataset.price), 0);
    let price = (base * Number(size.dataset.factor) + toppings) * Number(slider.value);
    if (promoApplied) {
      price *= 1 - PROMO_DISCOUNT;
    }
    return Math.round(price);
  }

  function render() {
    quantityOutput.value = slider.value;
    total.textContent = calcTotal() + ' ₽';
  }

  // The buttons change the slider from JS, which fires no input or change events.
  // A series of clicks ends with one change event, like releasing a dragged slider,
  // so the tracker sees the chosen quantity and not every step on the way.
  const QUANTITY_CHANGE_DELAY_MS = 500;
  let quantityTimer = 0;
  /** @type {string | null} slider value before the current series of clicks */
  let quantityBefore = null;

  /** @param {number} delta -1 or 1 */
  function stepQuantity(delta) {
    if (quantityBefore === null) {
      quantityBefore = slider.value;
    }
    slider.value = String(Number(slider.value) + delta); // clamped by min/max
    render();
    clearTimeout(quantityTimer);
    quantityTimer = setTimeout(commitQuantity, QUANTITY_CHANGE_DELAY_MS);
  }

  // Ends the series: fires change if the quantity differs from the one before it.
  function commitQuantity() {
    clearTimeout(quantityTimer);
    const changed = quantityBefore !== null && slider.value !== quantityBefore;
    quantityBefore = null;
    if (changed) {
      slider.dispatchEvent(new Event('change', { bubbles: true }));
    }
  }

  // A pending change must not be lost when the page is hidden or closed.
  window.addEventListener('pagehide', commitQuantity);
  document.addEventListener('visibilitychange', function () {
    if (document.visibilityState === 'hidden') {
      commitQuantity();
    }
  });

  form.addEventListener('input', render);
  form.addEventListener('change', render);

  minusButton.addEventListener('click', () => stepQuantity(-1));
  plusButton.addEventListener('click', () => stepQuantity(1));

  promoButton.addEventListener('click', function () {
    promoApplied = promoInput.value.trim().toUpperCase() === PROMO_CODE;
    promoStatus.textContent = promoApplied ? 'Скидка 10% применена' : 'Промокод не найден';
    promoStatus.classList.toggle('hint-error', !promoApplied);
    render();
  });

  resetButton.addEventListener('click', function () {
    form.reset(); // fires no change events, so commitQuantity sees the reset value
    promoApplied = false;
    promoStatus.textContent = '';
    orderStatus.textContent = '';
    render();
  });

  form.addEventListener('submit', function (e) {
    e.preventDefault(); // demo page: the order is not sent anywhere
    const name = nameInput.value.trim();
    const address = addressInput.value.trim();
    if (!name || !address) {
      orderStatus.textContent = 'Укажите имя и адрес доставки';
      orderStatus.classList.add('hint-error');
      return;
    }
    orderStatus.textContent = `Спасибо, ${name}! Заказ на ${total.textContent} принят.`;
    orderStatus.classList.remove('hint-error');
  });

  render();
})();
