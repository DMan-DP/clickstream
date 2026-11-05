// Demo page logic: price calculation and form handling. Tracking lives in tracker.js.
(function () {
  'use strict';

  const PROMO_CODE = 'PIZZA10';
  const PROMO_DISCOUNT = 0.1;

  const form = document.getElementById('order-form');
  const pizza = document.getElementById('pizza-select');
  const slider = document.getElementById('quantity-slider');
  const quantityOutput = document.getElementById('quantity-output');
  const promoInput = document.getElementById('promo-code');
  const promoStatus = document.getElementById('promo-status');
  const total = document.getElementById('order-total');
  const orderStatus = document.getElementById('order-status');

  let promoApplied = false;

  function calcTotal() {
    const base = Number(pizza.selectedOptions[0].dataset.price);
    const size = form.querySelector('input[name="size"]:checked');
    const toppings = Array.from(form.querySelectorAll('input[name="toppings"]:checked'))
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

  function stepQuantity(delta) {
    slider.value = Number(slider.value) + delta; // clamped by min/max
    render();
  }

  form.addEventListener('input', render);
  form.addEventListener('change', render);

  document.getElementById('quantity-minus-button').addEventListener('click', () => stepQuantity(-1));
  document.getElementById('quantity-plus-button').addEventListener('click', () => stepQuantity(1));

  document.getElementById('promo-apply-button').addEventListener('click', function () {
    promoApplied = promoInput.value.trim().toUpperCase() === PROMO_CODE;
    promoStatus.textContent = promoApplied ? 'Скидка 10% применена' : 'Промокод не найден';
    promoStatus.classList.toggle('hint-error', !promoApplied);
    render();
  });

  document.getElementById('reset-button').addEventListener('click', function () {
    form.reset();
    promoApplied = false;
    promoStatus.textContent = '';
    orderStatus.textContent = '';
    render();
  });

  form.addEventListener('submit', function (e) {
    e.preventDefault(); // demo page: the order is not sent anywhere
    const name = document.getElementById('customer-name').value.trim();
    const address = document.getElementById('delivery-address').value.trim();
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
